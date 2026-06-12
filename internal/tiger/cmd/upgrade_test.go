package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// setupUpgradeTest creates a temp config dir whose config.yaml points
// releases_url at the given URL, and returns the dir. Mirrors the env handling
// used elsewhere (TIGER_CONFIG_DIR + TIGER_ANALYTICS) so the analytics
// middleware and version-check post-run hook stay inert during tests.
func setupUpgradeTest(t *testing.T, releasesURL string) string {
	t.Helper()
	config.SetTestServiceName(t)

	tmpDir, err := os.MkdirTemp("", "tiger-test-upgrade-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	configContent := fmt.Sprintf("releases_url: %s\nanalytics: false\n", releasesURL)
	if err := os.WriteFile(config.GetConfigFile(tmpDir), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	t.Setenv("TIGER_ANALYTICS", "false")
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
		config.ResetGlobalConfig()
	})

	return tmpDir
}

func executeUpgradeCommand(ctx context.Context, configDir string, args ...string) (string, error) {
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(append([]string{"--config-dir", configDir}, args...))

	err = testRoot.Execute()
	return buf.String(), err
}

// startFakeReleasesServer mimics the release hosting used by scripts/install.sh
// (latest.txt). Only latest.txt is served; anything else returns 404.
func startFakeReleasesServer(t *testing.T, latestVersion string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if _, err := w.Write([]byte(latestVersion + "\n")); err != nil {
			t.Errorf("failed to write latest.txt response: %v", err)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestUpgradeCmd(t *testing.T) {
	releasesServer := startFakeReleasesServer(t, "v99.99.99")

	t.Run("rejects invalid --version", func(t *testing.T) {
		configDir := setupUpgradeTest(t, releasesServer.URL)
		_, err := executeUpgradeCommand(t.Context(), configDir, "upgrade", "--version", "not-a-version")
		wantErr := `invalid version "not-a-version": must be a valid semver version (e.g. v1.2.3)`
		if err == nil || err.Error() != wantErr {
			t.Errorf("got error %v, want %q", err, wantErr)
		}
	})

	t.Run("update alias rejects invalid --version", func(t *testing.T) {
		configDir := setupUpgradeTest(t, releasesServer.URL)
		_, err := executeUpgradeCommand(t.Context(), configDir, "update", "--version", "nope")
		wantErr := `invalid version "nope": must be a valid semver version (e.g. v1.2.3)`
		if err == nil || err.Error() != wantErr {
			t.Errorf("got error %v, want %q", err, wantErr)
		}
	})

	t.Run("refuses dev build without --force", func(t *testing.T) {
		// config.Version is "dev" in tests, so every invocation without --force
		// exercises the dev-build guard.
		configDir := setupUpgradeTest(t, releasesServer.URL)
		_, err := executeUpgradeCommand(t.Context(), configDir, "upgrade")
		wantErr := "tiger is a local dev build, not a released version; re-run with --force to replace it with version v99.99.99"
		if err == nil || err.Error() != wantErr {
			t.Errorf("got error %v, want %q", err, wantErr)
		}
	})

	t.Run("fails when latest version cannot be fetched", func(t *testing.T) {
		// Error from a network failure is non-deterministic (depends on the net
		// stack's exact wording), so we assert only the stable wrapping prefix.
		configDir := setupUpgradeTest(t, "http://127.0.0.1:1")
		_, err := executeUpgradeCommand(t.Context(), configDir, "upgrade")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantPrefix = "failed to check for latest version: "
		if !strings.HasPrefix(err.Error(), wantPrefix) {
			t.Errorf("unexpected error: %v (want prefix %q)", err, wantPrefix)
		}
	})
}

// TestUpgradeLiveCDNIntegration exercises the full upgrade flow end-to-end
// against the live release CDN: it builds a dev binary, runs
// `tiger upgrade --version <latest> --force` as a subprocess to replace that
// binary in place with the latest published release, and verifies the
// resulting binary runs and reports the new version.
//
// Gated behind TIGER_UPGRADE_INTEGRATION because it downloads a real release
// archive over the network. Enabled in the GitHub Actions test workflow so a
// broken upgrade path is caught before release.
func TestUpgradeLiveCDNIntegration(t *testing.T) {
	if os.Getenv("TIGER_UPGRADE_INTEGRATION") == "" {
		t.Skip("Skipping live upgrade integration test: set TIGER_UPGRADE_INTEGRATION=1 to run")
	}

	// Determine the latest published version, the same way install.sh does.
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(config.DefaultReleasesURL + "/latest.txt")
	if err != nil {
		t.Fatalf("failed to fetch latest.txt: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d fetching latest.txt", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read latest.txt: %v", err)
	}
	latestTag := normalizeTag(strings.TrimSpace(string(body)))

	// Build a dev binary to be upgraded in place. A dev build requires --force,
	// which is exactly the path this test wants to exercise.
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, binaryFilename())
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binPath, "github.com/timescale/tiger-cli/cmd/tiger")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Isolate the subprocesses from the developer's real config, and keep
	// analytics and the startup version check inert.
	configDir := filepath.Join(tmpDir, "config")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	env := append(os.Environ(),
		"TIGER_CONFIG_DIR="+configDir,
		"TIGER_ANALYTICS=false",
		"TIGER_VERSION_CHECK=false",
	)

	upgrade := exec.CommandContext(t.Context(), binPath, "upgrade", "--version", latestTag, "--force")
	upgrade.Env = env
	out, err := upgrade.CombinedOutput()
	if err != nil {
		t.Fatalf("upgrade failed: %v\n%s", err, out)
	}
	if want := "tiger upgraded successfully to " + latestTag; !strings.Contains(string(out), want) {
		t.Errorf("upgrade output missing %q:\n%s", want, out)
	}

	versionCmd := exec.CommandContext(t.Context(), binPath, "version")
	versionCmd.Env = env
	out, err = versionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version command on upgraded binary failed: %v\n%s", err, out)
	}
	if want := strings.TrimPrefix(latestTag, "v"); !strings.Contains(string(out), want) {
		t.Errorf("upgraded binary version output %q does not contain %q", out, want)
	}
}

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"1.2.3":  "v1.2.3",
		"v1.2.3": "v1.2.3",
		"v0.0.1": "v0.0.1",
	}
	for in, want := range cases {
		if got := normalizeTag(in); got != want {
			t.Errorf("normalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildReleaseArchiveName(t *testing.T) {
	name, isZip, err := buildReleaseArchiveName()
	if err != nil {
		// Unsupported platform in CI is acceptable; just ensure it's the
		// expected kind of failure rather than a panic.
		t.Skipf("unsupported platform for this test: %v", err)
	}

	if !strings.HasPrefix(name, "tiger-cli_") {
		t.Errorf("archive name %q does not start with project prefix \"tiger-cli_\"", name)
	}
	if runtime.GOOS == "windows" {
		if !isZip || !strings.HasSuffix(name, ".zip") {
			t.Errorf("expected zip archive on windows, got %q (isZip=%v)", name, isZip)
		}
	} else {
		if isZip || !strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("expected tar.gz archive, got %q (isZip=%v)", name, isZip)
		}
	}
}

func TestBinaryFilename(t *testing.T) {
	got := binaryFilename()
	want := "tiger"
	if runtime.GOOS == "windows" {
		want = "tiger.exe"
	}
	if got != want {
		t.Errorf("binaryFilename() = %q, want %q", got, want)
	}
}

// makeTarGz writes a gzipped tarball at path containing a single regular file
// named entryName with the given contents.
func makeTarGz(t *testing.T, path, entryName string, contents []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     entryName,
		Mode:     0o755,
		Size:     int64(len(contents)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(contents); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

func TestExtractBinaryFromArchive(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "tiger-cli_Test_x86_64.tar.gz")
	want := []byte("#!/bin/sh\necho fake tiger\n")
	makeTarGz(t, archivePath, "tiger", want)

	extracted, err := extractBinaryFromArchive(archivePath, false, "tiger")
	if err != nil {
		t.Fatalf("extractBinaryFromArchive: %v", err)
	}

	got, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted contents = %q, want %q", got, want)
	}

	t.Run("missing binary errors", func(t *testing.T) {
		if _, err := extractBinaryFromArchive(archivePath, false, "nonexistent"); err == nil {
			t.Error("expected error for missing binary, got nil")
		}
	})
}

func TestVerifyFileSHA256(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "blob")
	contents := []byte("hello tiger")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sum := sha256.Sum256(contents)
	hexSum := hex.EncodeToString(sum[:])

	if err := verifyFileSHA256(path, hexSum); err != nil {
		t.Errorf("verifyFileSHA256 with correct checksum: %v", err)
	}
	// Case-insensitive match should also succeed.
	if err := verifyFileSHA256(path, strings.ToUpper(hexSum)); err != nil {
		t.Errorf("verifyFileSHA256 with uppercase checksum: %v", err)
	}
	if err := verifyFileSHA256(path, "deadbeef"); err == nil {
		t.Error("expected checksum mismatch error, got nil")
	}
}

func TestFetchSHA256Checksum(t *testing.T) {
	mux := http.NewServeMux()
	// Bare digest.
	mux.HandleFunc("GET /bare.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "abc123")
	})
	// "<digest>  <filename>" form.
	mux.HandleFunc("GET /withname.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "abc123  tiger-cli_Linux_x86_64.tar.gz")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	for _, name := range []string{"bare", "withname"} {
		got, err := fetchSHA256Checksum(t.Context(), server.URL+"/"+name+".sha256")
		if err != nil {
			t.Fatalf("fetchSHA256Checksum(%s): %v", name, err)
		}
		if got != "abc123" {
			t.Errorf("fetchSHA256Checksum(%s) = %q, want %q", name, got, "abc123")
		}
	}

	if _, err := fetchSHA256Checksum(t.Context(), server.URL+"/missing.sha256"); err == nil {
		t.Error("expected error for 404 checksum, got nil")
	}
}

func TestReplaceRunningBinary(t *testing.T) {
	tmpDir := t.TempDir()

	// A stand-in for the currently running binary (never the real test binary).
	currentPath := filepath.Join(tmpDir, "tiger")
	if err := os.WriteFile(currentPath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	// The freshly extracted replacement.
	newPath := filepath.Join(tmpDir, "extracted-tiger")
	newContents := []byte("new binary")
	if err := os.WriteFile(newPath, newContents, 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	if err := replaceRunningBinary(currentPath, newPath); err != nil {
		t.Fatalf("replaceRunningBinary: %v", err)
	}

	got, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if !bytes.Equal(got, newContents) {
		t.Errorf("replaced contents = %q, want %q", got, newContents)
	}

	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat replaced binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Errorf("replaced binary is not executable: mode %v", info.Mode())
	}
}

func TestCheckCanReplaceBinary(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "tiger")
	if err := os.WriteFile(binPath, []byte("x"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	if err := checkCanReplaceBinary(binPath); err != nil {
		t.Errorf("checkCanReplaceBinary on writable dir: %v", err)
	}

	// Probe files must not be left behind.
	leftovers, _ := filepath.Glob(filepath.Join(tmpDir, ".tiger-upgrade-writecheck-*"))
	if len(leftovers) != 0 {
		t.Errorf("write-check probe files left behind: %v", leftovers)
	}
}
