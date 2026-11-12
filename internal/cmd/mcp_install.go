package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/pelletier/go-toml/v2/unstable/edit"
	"github.com/spf13/cobra"
	"github.com/tailscale/hujson"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/mcp"
	"github.com/timescale/tiger-cli/internal/util"
)

// mcpStartArgs are the arguments clients run the Tiger executable with to
// start the MCP server.
var mcpStartArgs = []string{"mcp", "start"}

// buildMCPInstallCmd creates the install subcommand for configuring editors
func buildMCPInstallCmd(_ *common.App) *cobra.Command {
	var noBackup bool
	var configPath string

	cmd := &cobra.Command{
		Use:     "install [client...]",
		Aliases: []string{"add"},
		Short:   "Install and configure Tiger MCP server for clients",
		Long: fmt.Sprintf(`Install and configure the Tiger MCP server for one or more MCP clients or AI
assistants.

The server is added to each client's user-level MCP configuration file, so it's
available in all projects. Other servers configured there are left alone.

%s
With no arguments, you're prompted to select clients interactively, starting
from the clients the server is already installed for.

Re-running the command replaces an existing Tiger MCP server entry with the
current one. Each configuration file is backed up before it's changed, unless
--no-backup is given.`, generateSupportedEditorsHelp()),
		Example: `  # Interactive client selection
  tiger mcp install

  # Install for Claude Code and Cursor
  tiger mcp install claude-code cursor

  # Install without creating a backup
  tiger mcp install claude-code --no-backup

  # Use a custom configuration file path
  tiger mcp install claude-code --config-path ~/custom/config.json`,
		ValidArgs:    getValidEditorNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" && len(args) != 1 {
				return errors.New("--config-path requires exactly one client argument")
			}

			var clients []*clientConfig
			var err error
			if len(args) > 0 {
				clients, err = mcpClientsFromArgs(args)
			} else {
				clients, err = mcpClientsFromPicker(cmd)
			}
			if err != nil {
				return err
			}

			command, err := getTigerExecutablePath()
			if err != nil {
				return err
			}

			// Prepare every install before writing anything, so a config file
			// that can't be read or parsed stops them all.
			installs := make([]mcpInstall, len(clients))
			for i, client := range clients {
				installs[i], err = prepareMCPInstall(client, configPath, mcp.ServerName, command, mcpStartArgs)
				if err != nil {
					return fmt.Errorf("failed to install for %s: %w", client.Name, err)
				}
			}
			for i, in := range installs {
				if err := in.write(!noBackup); err != nil {
					if i > 0 {
						printMCPInstalled(cmd, installs[:i])
					}
					return fmt.Errorf("failed to install for %s: %w", in.client.Name, err)
				}
			}

			printMCPInstalled(cmd, installs)
			cmd.Printf("\nRestart your coding agents to load the new configuration.\n")
			cmd.Printf("\nTry asking your AI assistant:\n")
			for _, prompt := range []string{
				"List my Tiger Cloud services",
				"Create a new database service called my-app-db",
				"Show me Tiger Cloud documentation about hypertables",
				"What are the best practices for PostgreSQL indexing?",
				"Help me optimize my PostgreSQL queries",
				"Help me create a new database schema for my application",
				"Help me figure out which tables should be hypertables",
			} {
				cmd.Printf("  %q\n", prompt)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Skip creating backup of existing configuration (default: create backup)")
	cmd.Flags().StringVar(&configPath, "config-path", "", "Custom path to configuration file (overrides default locations; requires a single client)")
	registerFlagCompletion(cmd, "config-path", fileCompletion())

	return cmd
}

// InstallOptions configures the MCP server installation behavior
type InstallOptions struct {
	// ClientName is the name of the client to configure (required)
	ClientName string
	// ServerName is the name to register the MCP server as (required)
	ServerName string
	// Command is the path to the MCP server binary (required)
	Command string
	// Args are the arguments to pass to the MCP server binary (required)
	Args []string
	// CreateBackup creates a backup of existing config files before modification
	CreateBackup bool
	// CustomConfigPath overrides the default config file location
	CustomConfigPath string
}

// ClientInfo contains information about a supported MCP client.
type ClientInfo struct {
	// Name is the human-readable display name (e.g., "Claude Code", "Cursor")
	Name string
	// ClientName is the identifier to use in InstallOptions.ClientName (e.g., "claude-code", "cursor")
	ClientName string
}

// SupportedClients returns information about all supported MCP clients.
func SupportedClients() []ClientInfo {
	clients := make([]ClientInfo, len(supportedClients))
	for i, c := range supportedClients {
		clients[i] = ClientInfo{
			Name:       c.Name,
			ClientName: c.EditorNames[0],
		}
	}
	return clients
}

// InstallMCPForClient installs an MCP server configuration for the specified client.
// This is a generic, configurable function exported for use by external projects via pkg/mcpinstall.
// Required options: ServerName, Command, Args must all be provided.
func InstallMCPForClient(opts InstallOptions) error {
	if opts.ClientName == "" {
		return errors.New("missing required option: ClientName")
	}
	if opts.ServerName == "" {
		return errors.New("missing required option: ServerName")
	}
	if opts.Command == "" {
		return errors.New("missing required option: Command")
	}
	if opts.Args == nil {
		return errors.New("missing required option: Args")
	}

	client, err := findClientConfig(opts.ClientName)
	if err != nil {
		return err
	}
	in, err := prepareMCPInstall(client, opts.CustomConfigPath, opts.ServerName, opts.Command, opts.Args)
	if err != nil {
		return err
	}
	return in.write(opts.CreateBackup)
}

// getValidEditorNames returns all valid client names from supportedClients
func getValidEditorNames() []string {
	var validNames []string
	for _, client := range supportedClients {
		validNames = append(validNames, client.EditorNames...)
	}
	return validNames
}

// findClientConfig finds the client configuration for a given client name,
// matched case-insensitively.
func findClientConfig(clientName string) (*clientConfig, error) {
	for i := range supportedClients {
		if slices.ContainsFunc(supportedClients[i].EditorNames, func(n string) bool { return strings.EqualFold(n, clientName) }) {
			return &supportedClients[i], nil
		}
	}
	return nil, fmt.Errorf("unsupported client: %s. Supported clients: %s", clientName, strings.Join(getValidEditorNames(), ", "))
}

// generateSupportedEditorsHelp generates the supported clients section for help text
func generateSupportedEditorsHelp() string {
	var result strings.Builder
	result.WriteString("Supported Clients:\n")
	for _, cfg := range supportedClients {
		// Show only the primary editor name in help text
		fmt.Fprintf(&result, "  %-24s Configure for %s\n", cfg.EditorNames[0], cfg.Name)
	}
	return result.String()
}

// mcpClientsFromArgs returns the clients the given names select, in
// supportedClients order and without duplicates.
func mcpClientsFromArgs(names []string) ([]*clientConfig, error) {
	selected := make([]bool, len(supportedClients))
	for _, name := range names {
		client, err := findClientConfig(name)
		if err != nil {
			return nil, err
		}
		selected[slices.IndexFunc(supportedClients, func(c clientConfig) bool { return c.ClientType == client.ClientType })] = true
	}
	var clients []*clientConfig
	for i := range supportedClients {
		if selected[i] {
			clients = append(clients, &supportedClients[i])
		}
	}
	return clients, nil
}

// mcpClientsFromPicker prompts the user to choose clients, starting from the
// ones the Tiger MCP server is already installed for, and returns the chosen
// ones. It fails without a terminal to prompt on.
func mcpClientsFromPicker(cmd *cobra.Command) ([]*clientConfig, error) {
	if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
		return nil, errors.New("TTY not detected - specify clients as arguments (e.g. 'tiger mcp install claude-code')")
	}
	items := make([]pickerItem, len(supportedClients))
	for i, c := range supportedClients {
		path, err := c.MCPConfig.resolvePath()
		if err != nil {
			return nil, fmt.Errorf("failed to determine configuration file for %s: %w", c.Name, err)
		}
		items[i] = pickerItem{
			label:    c.Name,
			detail:   displayPath(path),
			selected: hasMCPServer(path, c.MCPConfig, mcp.ServerName),
		}
	}
	header := ansi.Wordwrap("Select clients to install the Tiger MCP server for. Clients that already have it start out checked; unchecking one doesn't uninstall it.", 80, "")

	chosen, err := runPicker(cmd, header, items)
	if err != nil {
		return nil, err
	}
	var clients []*clientConfig
	for i := range supportedClients {
		if chosen[i] {
			clients = append(clients, &supportedClients[i])
		}
	}
	if len(clients) == 0 {
		return nil, errors.New("no clients selected")
	}
	return clients, nil
}

// hasMCPServer reports whether the config file at path, laid out as f
// describes, has an entry for the named server. Anything that can't be read or parsed counts as not
// installed: this only decides what the picker starts with, and installing
// would report the underlying problem.
func hasMCPServer(path string, f mcpConfigFile, name string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if f.isTOML() {
		doc, err := edit.Parse(data)
		return err == nil && doc.Has([]string{f.ServersKey, name})
	}
	doc, err := hujson.Parse(data)
	if err != nil {
		return false
	}
	root, ok := doc.Value.(*hujson.Object)
	if !ok {
		return false
	}
	servers := findJSONMember(root, f.ServersKey)
	if servers == nil {
		return false
	}
	obj, ok := servers.Value.(*hujson.Object)
	return ok && findJSONMember(obj, name) != nil
}

// printMCPInstalled prints the clients installed for and their config files.
func printMCPInstalled(cmd *cobra.Command, installs []mcpInstall) {
	width := 0
	for _, in := range installs {
		width = max(width, len(in.client.Name))
	}
	cmd.Printf("Installed Tiger MCP server for:\n")
	for _, in := range installs {
		cmd.Printf("  %-*s  %s\n", width, in.client.Name, in.path)
	}
}

// tigerExecutablePathFunc can be overridden in tests to return a fixed path
var tigerExecutablePathFunc = defaultGetTigerExecutablePath

// getTigerExecutablePath returns the full path to the currently executing Tiger binary
func getTigerExecutablePath() (string, error) {
	return tigerExecutablePathFunc()
}

// defaultGetTigerExecutablePath is the default implementation
func defaultGetTigerExecutablePath() (string, error) {
	tigerPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// If running via 'go run', os.Executable() returns a temp path like /tmp/go-build*/exe/tiger
	// In this case, return "tiger" assuming it's in PATH for development
	if strings.Contains(tigerPath, "go-build") && strings.Contains(tigerPath, "/exe/") {
		return "tiger", nil
	}

	return tigerPath, nil
}

// mcpInstall is an MCP server installation into one client's config file,
// prepared in memory and ready to write.
type mcpInstall struct {
	client  *clientConfig
	path    string // the config file, as displayed
	target  string // the file written: path with symlinks resolved
	mode    fs.FileMode
	content []byte // the new file content, or nil if the file is already up to date
}

// prepareMCPInstall reads client's config file (or customPath, if set) and
// prepares it with the named server's entry set to run command with args,
// replacing any existing entry of that name.
func prepareMCPInstall(client *clientConfig, customPath, serverName, command string, args []string) (mcpInstall, error) {
	in := mcpInstall{
		client: client,
		mode:   0o600,
	}
	var err error
	if customPath != "" {
		in.path, err = filepath.Abs(util.ExpandPath(customPath))
	} else {
		in.path, err = client.MCPConfig.resolvePath()
	}
	if err != nil {
		return mcpInstall{}, fmt.Errorf("failed to determine configuration file: %w", err)
	}

	// Write through a symlink rather than replacing it, so a config file
	// managed elsewhere (e.g. by a dotfiles repo) stays managed there.
	in.target = in.path
	if resolved, err := filepath.EvalSymlinks(in.path); err == nil {
		in.target = resolved
	} else if !errors.Is(err, fs.ErrNotExist) {
		return mcpInstall{}, fmt.Errorf("failed to resolve configuration file: %w", err)
	}

	data, err := os.ReadFile(in.target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return mcpInstall{}, fmt.Errorf("failed to read configuration file: %w", err)
	default:
		info, err := os.Stat(in.target)
		if err != nil {
			return mcpInstall{}, fmt.Errorf("failed to read configuration file: %w", err)
		}
		in.mode = info.Mode().Perm()
	}

	entry := client.MCPConfig.Entry
	entry.Command = command
	entry.Args = args
	if client.MCPConfig.isTOML() {
		in.content, err = setTOMLServer(data, client.MCPConfig.ServersKey, serverName, entry)
	} else {
		in.content, err = setJSONServer(data, client.MCPConfig.ServersKey, serverName, entry)
	}
	if err != nil {
		return mcpInstall{}, fmt.Errorf("failed to parse %s: %w", in.path, err)
	}
	return in, nil
}

// write writes the prepared config file, first backing up the existing one if
// backup is set. It does nothing if the file is already up to date.
func (in mcpInstall) write(backup bool) error {
	if in.content == nil {
		return nil
	}
	if backup {
		if _, err := createConfigBackup(in.target); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}
	if err := writeFileAtomic(in.target, in.content, in.mode); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	return nil
}

// setJSONServer returns data, a JSON config file (comments and trailing commas
// allowed), with servers[name] set to entry, or nil if it's already exactly
// that. Only the entry's own bytes change; a new one goes on its own line,
// indented like the rest of the file.
func setJSONServer(data []byte, serversKey, name string, entry any) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}\n")
	}
	doc, err := hujson.Parse(data)
	if err != nil {
		return nil, err
	}
	root, ok := doc.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("not a JSON object")
	}

	indent := "  "
	if len(root.Members) > 0 {
		if before := string(root.Members[0].Name.BeforeExtra); strings.Contains(before, "\n") {
			indent = before[strings.LastIndex(before, "\n")+1:]
		}
	}

	servers := findJSONMember(root, serversKey)
	if servers == nil {
		servers = appendJSONMember(root, serversKey, &hujson.Object{}, "", indent)
	}
	obj, ok := servers.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", serversKey)
	}

	entryJSON, err := json.MarshalIndent(entry, indent+indent, indent)
	if err != nil {
		return nil, err
	}
	value, err := hujson.Parse(entryJSON)
	if err != nil {
		return nil, err
	}
	if existing := findJSONMember(obj, name); existing != nil {
		if equalJSON(*existing, value) {
			return nil, nil
		}
		existing.Value = value.Value
	} else {
		appendJSONMember(obj, name, value.Value, indent, indent)
	}
	return doc.Pack(), nil
}

// findJSONMember returns the value of obj's member with the given name, or
// nil if there's none.
func findJSONMember(obj *hujson.Object, name string) *hujson.Value {
	for i := range obj.Members {
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.String() == name {
			return &obj.Members[i].Value
		}
	}
	return nil
}

// appendJSONMember adds a member to obj on its own line and returns its value.
// prefix is the indentation of obj's own line, and indent the file's
// indentation unit.
func appendJSONMember(obj *hujson.Object, name string, value hujson.ValueTrimmed, prefix, indent string) *hujson.Value {
	if len(obj.Members) == 0 {
		obj.AfterExtra = hujson.Extra("\n" + prefix)
	}
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: hujson.Extra("\n" + prefix + indent),
			Value:       hujson.String(name),
		},
		Value: hujson.Value{
			BeforeExtra: hujson.Extra(" "),
			Value:       value,
		},
	})
	return &obj.Members[len(obj.Members)-1].Value
}

// equalJSON reports whether a and b hold the same data, ignoring comments
// and formatting.
func equalJSON(a, b hujson.Value) bool {
	var decoded [2]any
	for i, v := range []hujson.Value{a, b} {
		v = v.Clone()
		v.Standardize()
		if err := json.Unmarshal(v.Pack(), &decoded[i]); err != nil {
			return false
		}
	}
	return reflect.DeepEqual(decoded[0], decoded[1])
}

// setTOMLServer returns data, a TOML config file, with [serversKey.name] set
// to entry, or nil if it's already exactly that. The rest of the file keeps
// its comments and layout.
func setTOMLServer(data []byte, serversKey, name string, entry any) ([]byte, error) {
	doc, err := edit.Parse(data)
	if err != nil {
		return nil, err
	}

	// The entry's fields, in order, decoded the way edit.Document.Get decodes
	// existing values so the two compare equal.
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	fields, err := hujson.Parse(entryJSON)
	if err != nil {
		return nil, err
	}
	want := map[string]any{}
	var keys []string
	for _, m := range fields.Value.(*hujson.Object).Members {
		key := m.Name.Value.(hujson.Literal).String()
		var value any
		if err := json.Unmarshal(m.Value.Pack(), &value); err != nil {
			return nil, err
		}
		keys = append(keys, key)
		want[key] = value
	}

	path := []string{serversKey, name}
	if existing, ok := doc.Get(path); ok && reflect.DeepEqual(existing, want) {
		return nil, nil
	}
	doc.Delete(path)
	for _, key := range keys {
		if err := doc.Set([]string{serversKey, name, key}, want[key]); err != nil {
			return nil, err
		}
	}
	return doc.Bytes(), nil
}

// createConfigBackup creates a backup of the existing configuration file and returns the backup path
func createConfigBackup(configPath string) (string, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); errors.Is(err, fs.ErrNotExist) {
		// No existing config file, no backup needed
		return "", nil
	}

	backupPath := fmt.Sprintf("%s.backup.%d", configPath, time.Now().UnixNano())

	// Get original file mode, fallback to 0600 if unavailable
	origInfo, err := os.Stat(configPath)
	var mode fs.FileMode = 0o600
	if err == nil {
		mode = origInfo.Mode().Perm()
	}

	// Read original file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original config file: %w", err)
	}

	// Write backup file
	if err := os.WriteFile(backupPath, data, mode); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

// writeFileAtomic writes data to path with the given permissions, creating
// its directory if needed. The data goes to a temporary file beside path that
// then replaces it, so a client reading the file never sees it half written.
func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tiger-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
