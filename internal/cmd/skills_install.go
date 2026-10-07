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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

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

	// maxSkillsArchiveSize bounds the decompressed archive, guarding against a
	// runaway download.
	maxSkillsArchiveSize = 256 << 20
	// maxSymlinkHops bounds symlink resolution within the archive, which also
	// breaks symlink cycles.
	maxSymlinkHops = 40
)

// skillsTarballURL is the GitHub REST API endpoint for the skills repo's
// tarball. GitHub answers it with a redirect to a short-lived download URL.
// It's a var so tests can point it at a local server.
var skillsTarballURL = "https://api.github.com/repos/" + skillsRepo + "/tarball/" + skillsRepoRef

// skillNamePattern matches valid skill names per the Agent Skills
// specification. Anything else is skipped, which also keeps every name safe to
// use as a path component.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func buildSkillsInstallCmd(_ *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install [client]",
		Aliases: []string{"add"},
		Short:   "Install agent skills for an AI coding agent",
		Long: fmt.Sprintf(`Install agent skills for an AI coding agent.

Skills are downloaded from https://github.com/%s and installed
for the current user into ~/.agents/skills, which most coding agents read. For
clients that read skills from their own directory, each skill is symlinked
into that directory, so every client shares a single copy.

Existing skills with the same names are replaced, so re-running the command
updates the installed skills to the latest version.

%s
If no client is specified, you'll be prompted to select one interactively.`, skillsRepo, generateSkillsClientsHelp()),
		Example: `  # Interactive client selection
  tiger skills install

  # Install for Claude Code
  tiger skills install claude-code

  # Install for Codex
  tiger skills install codex`,
		Args:         cobra.MaximumNArgs(1),
		ValidArgs:    getValidEditorNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var clientName string
			if len(args) == 0 {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return errors.New("TTY not detected - specify a client as an argument (e.g. 'tiger skills install claude-code')")
				}
				var err error
				clientName, err = selectClientInteractively(cmd, "Select a client to install skills for:")
				if err != nil {
					return fmt.Errorf("failed to select client: %w", err)
				}
			} else {
				clientName = args[0]
			}

			clientCfg, err := findClientConfig(clientName)
			if err != nil {
				return err
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to determine home directory: %w", err)
			}
			skillsDir := filepath.Join(home, ".agents", "skills")
			var linkDir string
			if clientCfg.skillsDir != nil {
				linkDir = clientCfg.skillsDir(home, os.Getenv)
			}

			skills, err := fetchSkills(cmd.Context())
			if err != nil {
				return err
			}

			result, err := installSkills(skills, skillsDir, linkDir)
			if err != nil {
				return err
			}

			cmd.Printf("Installed %d skills to %s:\n", len(skills), skillsDir)
			for _, s := range skills {
				cmd.Printf("  %s\n", s.name)
			}
			switch {
			case result.copied:
				cmd.Printf("Copied skills into %s (symlinks are not supported here).\n", linkDir)
			case result.linked:
				cmd.Printf("Linked skills into %s.\n", linkDir)
			}
			cmd.Printf("\nRestart %s to load the new skills.\n", clientCfg.Name)
			return nil
		},
	}

	return cmd
}

// generateSkillsClientsHelp generates the supported clients section of the
// help text, noting where each client's skills land. Env var overrides are
// ignored so the text doesn't depend on the environment it's generated in.
func generateSkillsClientsHelp() string {
	noEnv := func(string) string { return "" }
	var b strings.Builder
	b.WriteString("Supported Clients:\n")
	for _, cfg := range supportedClients {
		dir := "~/.agents/skills"
		if cfg.skillsDir != nil {
			dir = cfg.skillsDir("~", noEnv)
		}
		fmt.Fprintf(&b, "  %-24s %s (%s)\n", cfg.EditorNames[0], cfg.Name, dir)
	}
	return b.String()
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
	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found in %s", skillsRepo)
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
// SKILL.md. Symlinks within the repo are resolved to the files they point at,
// since skills share files that way and an installed skill must stand alone.
func extractSkills(r io.Reader) ([]skill, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	a := &skillsArchive{
		files: map[string]skillFile{},
		links: map[string]string{},
	}
	tr := tar.NewReader(io.LimitReader(gz, maxSkillsArchiveSize))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		// GitHub tarballs nest everything under a single top-level directory
		// named after the repo and commit. Entries outside it (the pax global
		// header) have no slash and are skipped.
		_, name, ok := strings.Cut(hdr.Name, "/")
		name = strings.TrimSuffix(name, "/")
		if !ok || !fs.ValidPath(name) || name == "." {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			a.files[name] = skillFile{
				executable: hdr.Mode&0o111 != 0,
				data:       data,
			}
		case tar.TypeSymlink:
			// Absolute targets point outside the repo, and relative ones may
			// climb out of it; either way they can't be resolved.
			if path.IsAbs(hdr.Linkname) {
				continue
			}
			a.links[name] = path.Join(path.Dir(name), hdr.Linkname)
		}
	}

	var skills []skill
	for _, name := range a.children(skillsRepoDir) {
		if !skillNamePattern.MatchString(name) {
			continue
		}
		s := skill{name: name}
		a.walk(path.Join(skillsRepoDir, name), "", 0, func(f skillFile) {
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

// skillsArchive indexes the regular files and symlinks of an extracted
// tarball by their slash-separated path within the repo.
type skillsArchive struct {
	files map[string]skillFile
	links map[string]string // symlink path -> target path
}

// walk emits every regular file reachable at src, which may be a file, a
// symlink, or a directory, with paths rebased from src onto dst. Dangling
// symlinks, and those nested more than maxSymlinkHops deep, are skipped.
func (a *skillsArchive) walk(src, dst string, hops int, emit func(skillFile)) {
	if f, ok := a.files[src]; ok {
		if dst != "" {
			f.path = dst
			emit(f)
		}
		return
	}
	if target, ok := a.links[src]; ok {
		if hops < maxSymlinkHops {
			a.walk(target, dst, hops+1, emit)
		}
		return
	}
	for _, child := range a.children(src) {
		a.walk(path.Join(src, child), path.Join(dst, child), hops, emit)
	}
}

// children returns the sorted names of the entries directly inside dir.
func (a *skillsArchive) children(dir string) []string {
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

// installSkillsResult reports how skills reached the client's own skills
// directory, if it has one.
type installSkillsResult struct {
	linked bool // symlinked into the client's directory
	copied bool // copied into the client's directory, as symlinks failed
}

// installSkills writes each skill into skillsDir, replacing any existing copy,
// then symlinks it into linkDir if set. Linking is skipped when linkDir
// resolves to skillsDir itself (e.g. the user has symlinked one to the other).
func installSkills(skills []skill, skillsDir, linkDir string) (installSkillsResult, error) {
	var result installSkillsResult
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return result, fmt.Errorf("failed to create skills directory: %w", err)
	}
	for _, s := range skills {
		if err := writeSkill(filepath.Join(skillsDir, s.name), s); err != nil {
			return result, fmt.Errorf("failed to install skill %s: %w", s.name, err)
		}
	}

	if linkDir == "" {
		return result, nil
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		return result, fmt.Errorf("failed to create skills directory: %w", err)
	}
	same, err := sameDir(skillsDir, linkDir)
	if err != nil {
		return result, err
	}
	if same {
		return result, nil
	}
	for _, s := range skills {
		target := filepath.Join(skillsDir, s.name)
		link := filepath.Join(linkDir, s.name)
		err := linkSkill(target, link)
		if errors.Is(err, errSymlinkUnsupported) {
			err = writeSkill(link, s)
			result.copied = true
		} else {
			result.linked = true
		}
		if err != nil {
			return result, fmt.Errorf("failed to link skill %s: %w", s.name, err)
		}
	}
	return result, nil
}

// errSymlinkUnsupported reports that a symlink couldn't be created (e.g. on
// Windows without Developer Mode), so the caller should fall back to a copy.
var errSymlinkUnsupported = errors.New("symlinks not supported")

// writeSkill writes s to dir, replacing whatever is there. The new copy is
// built in a temporary directory beside dir and swapped in, so a failure
// partway through leaves the existing copy intact.
func writeSkill(dir string, s skill) error {
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".tiger-"+s.name+"-")
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
	// MkdirTemp creates the directory 0700; skills are ordinary user files.
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	return replacePath(tmp, dir)
}

// linkSkill points link at target, replacing whatever is at link unless it's
// already the right symlink. The symlink is created under a temporary name
// and swapped in, so link is never left missing.
func linkSkill(target, link string) error {
	if current, err := os.Readlink(link); err == nil && current == target {
		return nil
	}

	tmp, err := os.MkdirTemp(filepath.Dir(link), ".tiger-"+filepath.Base(link)+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	tmpLink := filepath.Join(tmp, "link")
	if err := os.Symlink(target, tmpLink); err != nil {
		return fmt.Errorf("%w: %w", errSymlinkUnsupported, err)
	}
	return replacePath(tmpLink, link)
}

// replacePath moves src to dst, replacing any file, directory, or symlink at
// dst. A rename can't replace a non-empty directory, so the existing entry is
// first moved aside, and restored if the swap fails. Symlinks are moved and
// removed as links, never followed.
func replacePath(src, dst string) error {
	if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
		return os.Rename(src, dst)
	} else if err != nil {
		return err
	}

	aside, err := os.MkdirTemp(filepath.Dir(dst), ".tiger-old-")
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
	return nil
}

// sameDir reports whether a and b resolve to the same directory once symlinks
// are followed.
func sameDir(a, b string) (bool, error) {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false, err
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false, err
	}
	return ra == rb, nil
}
