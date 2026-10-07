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

func buildSkillsInstallCmd(_ *common.App) *cobra.Command {
	var force bool
	var skillsDirFlag string

	cmd := &cobra.Command{
		Use:     "install [client]",
		Aliases: []string{"add"},
		Short:   "Install agent skills for an AI coding agent",
		Long: fmt.Sprintf(`Install agent skills for an AI coding agent.

Skills are installed for the current user into ~/.agents/skills, which most
coding agents read, or into the client's own skills directory for clients that
don't.

Re-running the command updates the installed skills to the latest version and
removes any that are no longer available. Existing skills that weren't
installed by Tiger CLI are never replaced unless --force is given.

%s
If no client is specified, you'll be prompted to select one interactively.`, generateSkillsClientsHelp()),
		Example: `  # Interactive client selection
  tiger skills install

  # Install for Claude Code
  tiger skills install claude-code

  # Install for Codex
  tiger skills install codex

  # Install into a custom skills directory
  tiger skills install claude-code --skills-dir ~/my-skills`,
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

			skillsDir, err := resolveSkillsDir(clientCfg, skillsDirFlag)
			if err != nil {
				return err
			}

			skills, err := fetchSkills(cmd.Context())
			if err != nil {
				return err
			}

			removed, err := installSkills(skills, skillsDir, force)
			if err != nil {
				return err
			}

			cmd.Printf("Installed %d skills to %s:\n", len(skills), skillsDir)
			for _, s := range skills {
				cmd.Printf("  %s\n", s.name)
			}
			for _, name := range removed {
				cmd.Printf("Removed %s (no longer available).\n", name)
			}
			cmd.Printf("\nRestart %s to load the new skills.\n", clientCfg.Name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Replace existing skills that weren't installed by Tiger CLI")
	cmd.Flags().StringVar(&skillsDirFlag, "skills-dir", "", "Custom skills directory to install into (overrides the client's default)")
	registerFlagCompletion(cmd, "skills-dir", dirCompletion)

	return cmd
}

// generateSkillsClientsHelp generates the supported clients section of the
// help text, noting where each client's skills land.
func generateSkillsClientsHelp() string {
	var b strings.Builder
	b.WriteString("Supported Clients:\n")
	for _, cfg := range supportedClients {
		fmt.Fprintf(&b, "  %-24s %s (%s)\n", cfg.EditorNames[0], cfg.Name, cfg.SkillsDir)
	}
	return b.String()
}

// resolveSkillsDir returns the absolute directory to install skills into for
// the given client, or custom (relative to the working directory) if set.
func resolveSkillsDir(cfg *clientConfig, custom string) (string, error) {
	var dir string
	if custom != "" {
		abs, err := filepath.Abs(util.ExpandPath(custom))
		if err != nil {
			return "", fmt.Errorf("failed to resolve skills directory: %w", err)
		}
		return abs, nil
	}
	if expanded, ok := expandEnvStrict(cfg.SkillsDirEnv); cfg.SkillsDirEnv != "" && ok {
		dir = filepath.Clean(expanded)
	} else {
		dir = util.ExpandPath(cfg.SkillsDir)
	}
	// ExpandPath leaves a ~ in place if the home directory is unknown, which
	// would otherwise install relative to the working directory.
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("failed to determine skills directory: %s is not an absolute path", dir)
	}
	return dir, nil
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

// installSkills writes each skill into skillsDir, replacing any existing copy,
// then removes skills Tiger CLI installed earlier that are no longer among
// them, returning their names. Unless force is set, it refuses up front to
// replace anything Tiger CLI didn't install.
func installSkills(skills []skill, skillsDir string, force bool) ([]string, error) {
	if !force {
		var conflicts []string
		for _, s := range skills {
			exists, owned, err := skillOwnership(filepath.Join(skillsDir, s.name))
			if err != nil {
				return nil, fmt.Errorf("failed to check existing skill %s: %w", s.name, err)
			}
			if exists && !owned {
				conflicts = append(conflicts, s.name)
			}
		}
		if len(conflicts) > 0 {
			return nil, fmt.Errorf("skills already exist in %s and weren't installed by Tiger CLI: %s. Use --force to replace them",
				skillsDir, strings.Join(conflicts, ", "))
		}
	}

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
