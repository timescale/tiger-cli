package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

const (
	skillsRepo    = "timescale/pg-aiguide"
	skillsRepoRef = "main"
	// skillsRepoDir is the directory within the repo holding one subdirectory
	// per skill.
	skillsRepoDir = "skills"

	// skillMarkerFile is written into every skill Tiger CLI installs, marking
	// it as one a later install may replace or remove.
	skillMarkerFile    = ".tiger-cli"
	skillMarkerContent = "This skill is managed by Tiger CLI. Running 'tiger skills install' updates it\n" +
		"to the latest version, or removes it if it's no longer available.\n"

	// skillsTempPrefix starts the name of every temporary directory an install
	// creates in the skills directory.
	skillsTempPrefix = ".tiger-"
)

// skillsTarballURL is the GitHub REST API endpoint for the skills repo's
// tarball. GitHub answers it with a redirect to a short-lived download URL.
// It's a var so tests can point it at a local server.
var skillsTarballURL = "https://api.github.com/repos/" + skillsRepo + "/tarball/" + skillsRepoRef

// Colors for the install location picker, taken from the terminal theme.
// Bright colors and bold are avoided because some themes render them as greys,
// and dim text uses the faint attribute so it suits light and dark themes
// alike. They honor color.NoColor, which the command lifecycle sets from the
// color config option.
var (
	skillsPickerCursorColor = color.New(color.FgBlue)
	skillsPickerCheckColor  = color.New(color.FgGreen)
	skillsPickerDimColor    = color.New(color.Faint)
)

func buildSkillsInstallCmd(_ *common.App) *cobra.Command {
	var force bool
	var skillsDirFlag string

	cmd := &cobra.Command{
		Use:     "install [client...]",
		Aliases: []string{"add"},
		Short:   "Install agent skills for AI coding agents",
		Long: fmt.Sprintf(`Install agent skills for AI coding agents.

Skills are installed for the current user into one or more install locations:
the universal ~/.agents/skills directory, which most coding agents read, and
the skills directories of clients that don't read it. Each argument selects a
location, either by name or by the name of a client that reads it.

%s
With no arguments, you're prompted to select locations interactively, starting
from the locations Tiger CLI has installed skills to before.

Re-running the command updates the installed skills to the latest version and
removes any that are no longer available. Existing skills that weren't
installed by Tiger CLI are never replaced unless --force is given.`, generateSkillsTargetsHelp()),
		Example: `  # Interactive selection
  tiger skills install

  # Install to ~/.agents/skills and for Claude Code
  tiger skills install universal claude-code

  # Install into a custom skills directory
  tiger skills install --skills-dir ~/my-skills`,
		ValidArgs:    skillsTargetNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skillsDirFlag != "" && len(args) > 0 {
				return errors.New("--skills-dir can't be combined with client arguments")
			}

			var dirs []string
			switch {
			case skillsDirFlag != "":
				dir, err := filepath.Abs(util.ExpandPath(skillsDirFlag))
				if err != nil {
					return fmt.Errorf("failed to resolve skills directory: %w", err)
				}
				dirs = []string{dir}
			case len(args) > 0:
				targets, err := findSkillsTargets(args)
				if err != nil {
					return err
				}
				if dirs, err = resolveSkillsTargetDirs(targets); err != nil {
					return err
				}
			default:
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return errors.New("TTY not detected - specify install locations as arguments (e.g. 'tiger skills install universal')")
				}
				targets := skillsTargets()
				targetDirs, err := resolveSkillsTargetDirs(targets)
				if err != nil {
					return err
				}
				selected, err := preselectSkillsTargets(targetDirs)
				if err != nil {
					return err
				}
				if selected, err = selectSkillsTargets(cmd, targets, selected); err != nil {
					return err
				}
				for i, dir := range targetDirs {
					if selected[i] {
						dirs = append(dirs, dir)
					}
				}
				if len(dirs) == 0 {
					return errors.New("no install locations selected")
				}
			}

			skills, err := fetchSkills(cmd.Context())
			if err != nil {
				return err
			}

			if !force {
				if err := checkSkillConflicts(skills, dirs); err != nil {
					return err
				}
			}

			removed := make([][]string, len(dirs))
			for i, dir := range dirs {
				if removed[i], err = installSkills(skills, dir); err != nil {
					return err
				}
			}

			cmd.Printf("Installed %d skills:\n", len(skills))
			for _, s := range skills {
				cmd.Printf("  %s\n", s.name)
			}
			cmd.Printf("\nInstalled to:\n")
			for _, dir := range dirs {
				cmd.Printf("  %s\n", dir)
			}
			for i, dir := range dirs {
				if len(removed[i]) == 0 {
					continue
				}
				cmd.Printf("\nRemoved from %s (no longer available):\n", dir)
				for _, name := range removed[i] {
					cmd.Printf("  %s\n", name)
				}
			}
			cmd.Printf("\nRestart your coding agents to load the new skills.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Replace existing skills that weren't installed by Tiger CLI")
	cmd.Flags().StringVar(&skillsDirFlag, "skills-dir", "", "Install into this skills directory only (not remembered for later installs)")
	registerFlagCompletion(cmd, "skills-dir", dirCompletion)

	return cmd
}

// skillsTarget is a location skills can be installed to: the universal
// directory most clients read, or the skills directory of a client that
// doesn't read it.
type skillsTarget struct {
	label   string   // display name
	names   []string // names that select it on the command line; the first is canonical
	readers []string // display names of the clients that read it
	dir     string   // see clientConfig.SkillsDir
	dirEnv  string   // see clientConfig.SkillsDirEnv
}

// generateSkillsTargetsHelp generates the install locations section of the
// help text.
func generateSkillsTargetsHelp() string {
	var b strings.Builder
	b.WriteString("Install locations:\n")
	for _, t := range skillsTargets() {
		fmt.Fprintf(&b, "  %-24s %s (%s)\n", t.names[0], t.dir, strings.Join(t.readers, ", "))
	}
	return b.String()
}

// findSkillsTargets returns the install locations the given names select, in
// skillsTargets order and without duplicates. Names match case-insensitively.
func findSkillsTargets(names []string) ([]skillsTarget, error) {
	targets := skillsTargets()
	selected := make([]bool, len(targets))
	for _, name := range names {
		i := slices.IndexFunc(targets, func(t skillsTarget) bool {
			return slices.ContainsFunc(t.names, func(n string) bool { return strings.EqualFold(n, name) })
		})
		if i < 0 {
			return nil, fmt.Errorf("unsupported client: %s. Supported clients: %s", name, strings.Join(skillsTargetNames(), ", "))
		}
		selected[i] = true
	}
	var result []skillsTarget
	for i, t := range targets {
		if selected[i] {
			result = append(result, t)
		}
	}
	return result, nil
}

// skillsTargetNames returns every name that selects an install location.
func skillsTargetNames() []string {
	var names []string
	for _, t := range skillsTargets() {
		names = append(names, t.names...)
	}
	return names
}

// skillsTargets returns every install location: universal first, then each
// client with a skills directory of its own, in supportedClients order.
func skillsTargets() []skillsTarget {
	targets := []skillsTarget{{
		label: "Universal",
		names: []string{"universal"},
		dir:   "~/.agents/skills",
	}}
	for _, c := range supportedClients {
		if c.SkillsDir == "" {
			targets[0].names = append(targets[0].names, c.EditorNames...)
			targets[0].readers = append(targets[0].readers, c.Name)
			continue
		}
		targets = append(targets, skillsTarget{
			label:   c.Name,
			names:   c.EditorNames,
			readers: []string{c.Name},
			dir:     c.SkillsDir,
			dirEnv:  c.SkillsDirEnv,
		})
	}
	return targets
}

// preselectSkillsTargets reports which of dirs the picker starts with
// selected: those holding skills Tiger CLI installed, or the first (universal)
// if none do.
func preselectSkillsTargets(dirs []string) ([]bool, error) {
	selected := make([]bool, len(dirs))
	found := false
	for i, dir := range dirs {
		has, err := hasInstalledSkills(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to check for installed skills: %w", err)
		}
		selected[i] = has
		found = found || has
	}
	if !found {
		selected[0] = true
	}
	return selected, nil
}

// hasInstalledSkills reports whether skillsDir holds any skill Tiger CLI
// installed.
func hasInstalledSkills(skillsDir string) (bool, error) {
	entries, err := os.ReadDir(skillsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), skillsTempPrefix) {
			continue
		}
		_, owned, err := skillOwnership(filepath.Join(skillsDir, e.Name()))
		if err != nil {
			return false, err
		}
		if owned {
			return true, nil
		}
	}
	return false, nil
}

// skillsPickerItem is one install location offered by the picker.
type skillsPickerItem struct {
	label    string
	dir      string // as displayed
	selected bool
}

// selectSkillsTargets prompts the user to choose install locations, starting
// from the given selection, and returns the chosen selection. It's a var so
// command tests can stub the interactive picker.
var selectSkillsTargets = func(cmd *cobra.Command, targets []skillsTarget, selected []bool) ([]bool, error) {
	dirs, err := resolveSkillsTargetDirs(targets)
	if err != nil {
		return nil, err
	}
	items := make([]skillsPickerItem, len(targets))
	for i, t := range targets {
		items[i] = skillsPickerItem{
			label:    t.label,
			dir:      displayPath(dirs[i]),
			selected: selected[i],
		}
	}

	model := skillsPickerModel{
		header: ansi.Wordwrap(fmt.Sprintf("Select where to install agent skills. %s installs to %s, which %s read.",
			targets[0].label, items[0].dir, joinWithAnd(append(slices.Clone(targets[0].readers), "many other agents"))), 80, ""),
		items: items,
	}
	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
	final, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run install location selection: %w", err)
	}

	result := final.(skillsPickerModel)
	if !result.confirmed {
		return nil, errors.New("installation cancelled")
	}
	chosen := make([]bool, len(result.items))
	for i, item := range result.items {
		chosen[i] = item.selected
	}
	return chosen, nil
}

// resolveSkillsTargetDirs returns the absolute directory of each target.
func resolveSkillsTargetDirs(targets []skillsTarget) ([]string, error) {
	dirs := make([]string, len(targets))
	for i, t := range targets {
		var dir string
		if expanded, ok := expandEnvStrict(t.dirEnv); t.dirEnv != "" && ok {
			dir = filepath.Clean(expanded)
		} else {
			dir = util.ExpandPath(t.dir)
		}
		// ExpandPath leaves a ~ in place if the home directory is unknown,
		// which would otherwise install relative to the working directory.
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("failed to determine skills directory: %s is not an absolute path", dir)
		}
		dirs[i] = dir
	}
	return dirs, nil
}

// expandEnvStrict expands env var references in s, reporting false if any
// referenced variable is unset or empty.
func expandEnvStrict(s string) (string, bool) {
	ok := true
	expanded := os.Expand(s, func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			ok = false
		}
		return v
	})
	return expanded, ok
}

// skillsPickerModel is the Bubble Tea model for the install location picker.
type skillsPickerModel struct {
	header    string
	items     []skillsPickerItem
	cursor    int
	confirmed bool
}

func (m skillsPickerModel) Init() tea.Cmd {
	return nil
}

func (m skillsPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch k := key.String(); k {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "enter":
		m.confirmed = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "space":
		m.items[m.cursor].selected = !m.items[m.cursor].selected
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k[0] - '1'); i < len(m.items) {
			m.cursor = i
			m.items[i].selected = !m.items[i].selected
		}
	}
	return m, nil
}

func (m skillsPickerModel) View() tea.View {
	if m.confirmed {
		return tea.NewView("")
	}
	width := 0
	for _, item := range m.items {
		width = max(width, len(item.label))
	}

	var b strings.Builder
	b.WriteString(m.header + "\n\n")
	for i, item := range m.items {
		cursor := " "
		if m.cursor == i {
			cursor = skillsPickerCursorColor.Sprint(">")
		}
		check := " "
		if item.selected {
			check = skillsPickerCheckColor.Sprint("✓")
		}
		fmt.Fprintf(&b, "%s [%s] %d. %-*s  %s\n", cursor, check, i+1, width, item.label, skillsPickerDimColor.Sprint(item.dir))
	}
	b.WriteString("\n" + skillsPickerDimColor.Sprint("Use ↑/↓ to navigate, space or number keys to toggle, enter to confirm, q to quit"))
	return tea.NewView(b.String())
}

// displayPath abbreviates the home directory in path to ~.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rel, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rel)
	}
	return path
}

// joinWithAnd joins items into an English list ("a, b, and c").
func joinWithAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

// skill is one skill directory, with its contents held in memory.
type skill struct {
	name  string
	files []skillFile
}

// skillFile is a regular file within a skill, with symlinks already resolved.
type skillFile struct {
	path       string // slash-separated, relative to the skill directory
	executable bool
	data       []byte
}

// fetchSkills downloads the skills repo's tarball from GitHub and extracts the
// skills from it.
func fetchSkills(ctx context.Context) ([]skill, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, skillsTarballURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download skills: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, skillsDownloadError(resp)
	}

	skills, err := extractSkills(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read skills archive: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("failed to read skills archive: %w", err)
	}
	if len(skills) == 0 {
		return nil, errors.New("no skills found")
	}
	return skills, nil
}

// skillsDownloadError describes a failed tarball request, calling out GitHub's
// rate limit for unauthenticated requests.
func skillsDownloadError(resp *http.Response) error {
	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return fmt.Errorf("failed to download skills: GitHub API rate limit exceeded. Try again after %s",
				time.Unix(reset, 0).Local().Format("15:04 MST"))
		}
		return errors.New("failed to download skills: GitHub API rate limit exceeded. Try again later")
	}
	return fmt.Errorf("failed to download skills: unexpected status from GitHub: %s", resp.Status)
}

// extractSkills reads a gzipped tarball of the skills repo and returns its
// skills, sorted by name. A skill is a subdirectory of skillsRepoDir holding a
// SKILL.md. Symlinks within skillsRepoDir are resolved to the files they point
// at, since skills share files that way and an installed skill must stand
// alone; anything else in the repo is ignored.
func extractSkills(r io.Reader) ([]skill, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	a := &repoArchive{
		files: map[string]archiveFile{},
		links: map[string]string{},
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		// GitHub tarballs nest everything under a single top-level directory
		// named after the repo and commit. ValidPath keeps names safe to
		// join onto the install directory later.
		_, name, _ := strings.Cut(hdr.Name, "/")
		if !strings.HasPrefix(name, skillsRepoDir+"/") || !fs.ValidPath(name) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			a.files[name] = archiveFile{
				executable: hdr.Mode&0o111 != 0,
				data:       data,
			}
		case tar.TypeSymlink:
			// An absolute target can't refer to a file in the repo.
			if path.IsAbs(hdr.Linkname) {
				continue
			}
			a.links[name] = path.Join(path.Dir(name), hdr.Linkname)
		}
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	var skills []skill
	for _, name := range a.children(skillsRepoDir) {
		s := skill{name: name}
		a.walk(path.Join(skillsRepoDir, name), "", map[string]bool{}, func(f skillFile) {
			s.files = append(s.files, f)
		})
		if !slices.ContainsFunc(s.files, func(f skillFile) bool { return f.path == "SKILL.md" }) {
			continue
		}
		slices.SortFunc(s.files, func(a, b skillFile) int { return strings.Compare(a.path, b.path) })
		skills = append(skills, s)
	}
	return skills, nil
}

// repoArchive indexes the regular files and symlinks under skillsRepoDir in
// the repo's tarball by their slash-separated path within the repo.
type repoArchive struct {
	files map[string]archiveFile
	links map[string]string // symlink path -> target path
}

// archiveFile is a regular file in the repo's tarball.
type archiveFile struct {
	executable bool
	data       []byte
}

// walk emits every regular file reachable at src, which may be a file, a
// symlink, or a directory, with paths rebased from src onto dst. following
// holds the symlinks being resolved on the way to src, so a symlink that leads
// back to one of them (a cycle) is skipped, as are dangling symlinks.
func (a *repoArchive) walk(src, dst string, following map[string]bool, emit func(skillFile)) {
	if f, ok := a.files[src]; ok {
		if dst != "" {
			emit(skillFile{
				path:       dst,
				executable: f.executable,
				data:       f.data,
			})
		}
		return
	}
	if target, ok := a.links[src]; ok {
		if !following[src] {
			following[src] = true
			a.walk(target, dst, following, emit)
			delete(following, src)
		}
		return
	}
	for _, child := range a.children(src) {
		a.walk(path.Join(src, child), path.Join(dst, child), following, emit)
	}
}

// children returns the sorted names of the entries directly inside dir.
func (a *repoArchive) children(dir string) []string {
	prefix := dir + "/"
	seen := map[string]bool{}
	var names []string
	add := func(p string) {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			return
		}
		child, _, _ := strings.Cut(rest, "/")
		if !seen[child] {
			seen[child] = true
			names = append(names, child)
		}
	}
	for p := range a.files {
		add(p)
	}
	for p := range a.links {
		add(p)
	}
	slices.Sort(names)
	return names
}

// checkSkillConflicts returns an error naming every place in dirs where a
// skill would replace something Tiger CLI didn't install.
func checkSkillConflicts(skills []skill, dirs []string) error {
	var conflicts []string
	for _, dir := range dirs {
		for _, s := range skills {
			p := filepath.Join(dir, s.name)
			exists, owned, err := skillOwnership(p)
			if err != nil {
				return fmt.Errorf("failed to check existing skill %s: %w", p, err)
			}
			if exists && !owned {
				conflicts = append(conflicts, p)
			}
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("skills already exist and weren't installed by Tiger CLI: %s. Use --force to replace them",
			strings.Join(conflicts, ", "))
	}
	return nil
}

// installSkills writes each skill into skillsDir, replacing any existing copy,
// then removes skills Tiger CLI installed earlier that are no longer among
// them, returning their names.
func installSkills(skills []skill, skillsDir string) ([]string, error) {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}
	for _, s := range skills {
		if err := writeSkill(filepath.Join(skillsDir, s.name), s); err != nil {
			return nil, fmt.Errorf("failed to install skill %s: %w", s.name, err)
		}
	}

	removed, err := removeStaleSkills(skills, skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to remove skills that are no longer available: %w", err)
	}
	return removed, nil
}

// writeSkill writes s to dir along with the marker file, replacing whatever
// is there. The new copy is built in a temporary directory beside dir and
// swapped in, so a failure partway through leaves the existing copy intact.
func writeSkill(dir string, s skill) error {
	tmp, err := os.MkdirTemp(filepath.Dir(dir), skillsTempPrefix+s.name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	for _, f := range s.files {
		p := filepath.Join(tmp, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if f.executable {
			mode = 0o755
		}
		if err := os.WriteFile(p, f.data, mode); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, skillMarkerFile), []byte(skillMarkerContent), 0o644); err != nil {
		return err
	}
	// MkdirTemp creates the directory 0700; skills are ordinary user files.
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := replacePath(tmp, dir); err != nil {
		return err
	}
	return os.RemoveAll(tmp)
}

// replacePath moves src to dst, replacing any file, directory, or symlink at
// dst. A rename can't replace a non-empty directory, so the existing entry is
// first moved aside, and restored if the swap fails. A symlink at dst is
// removed as a link, never followed.
func replacePath(src, dst string) error {
	if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
		return os.Rename(src, dst)
	} else if err != nil {
		return err
	}

	aside, err := os.MkdirTemp(filepath.Dir(dst), skillsTempPrefix+"old-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(aside)

	old := filepath.Join(aside, "old")
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		if restoreErr := os.Rename(old, dst); restoreErr != nil {
			return fmt.Errorf("%w (and failed to restore %s: %w)", err, dst, restoreErr)
		}
		return err
	}
	return os.RemoveAll(aside)
}

// removeStaleSkills removes the skills in skillsDir that Tiger CLI installed
// but that aren't among skills, returning their names. It also quietly removes
// temporary directories left behind by an install that was killed partway
// through.
func removeStaleSkills(skills []skill, skillsDir string) ([]string, error) {
	current := make(map[string]bool, len(skills))
	for _, s := range skills {
		current[s.name] = true
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if !e.IsDir() || current[e.Name()] {
			continue
		}
		dir := filepath.Join(skillsDir, e.Name())
		if strings.HasPrefix(e.Name(), skillsTempPrefix) {
			if err := os.RemoveAll(dir); err != nil {
				return nil, err
			}
			continue
		}
		_, owned, err := skillOwnership(dir)
		if err != nil {
			return nil, err
		}
		if !owned {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		removed = append(removed, e.Name())
	}
	return removed, nil
}

// skillOwnership reports whether anything exists at dir, and whether it's a
// skill Tiger CLI installed: a directory (not a symlink, since Tiger CLI never
// creates those) holding the marker file.
func skillOwnership(dir string) (exists, owned bool, err error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	if !info.IsDir() {
		return true, false, nil
	}
	info, err = os.Lstat(filepath.Join(dir, skillMarkerFile))
	if errors.Is(err, fs.ErrNotExist) {
		return true, false, nil
	} else if err != nil {
		return true, false, err
	}
	return true, info.Mode().IsRegular(), nil
}
