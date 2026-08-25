package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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

	"github.com/timescale/tiger-cli/internal/config"
)

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

// fakeRelease describes the release served by startReleaseServer.
type fakeRelease struct {
	latest     string // latest.txt body
	tag        string // release tag in the download path (e.g. "v1.2.3")
	entryName  string // filename inside the archive
	contents   []byte // contents of that file
	checksum   string // .sha256 body; "" serves the archive's correct digest
	noChecksum bool   // don't serve the .sha256 route at all
}

// startReleaseServer serves latest.txt plus a single release archive for the
// current platform and its checksum. The default checksum body uses
// GoReleaser's "<digest>  <filename>" form with an uppercased digest, so the
// success path also covers checksum parsing and case-insensitive comparison.
// Returns the server and the archive's correct hex digest.
func startReleaseServer(t *testing.T, r fakeRelease) (*httptest.Server, string) {
	t.Helper()
	archiveName, isZip, err := buildReleaseArchiveName()
	if err != nil {
		t.Skipf("unsupported platform for release archives: %v", err)
	}
	archive := makeArchive(t, isZip, r.entryName, r.contents)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])

	checksumBody := r.checksum
	if checksumBody == "" {
		checksumBody = strings.ToUpper(digest) + "  " + archiveName
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest.txt", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, r.latest)
	})
	mux.HandleFunc("GET /releases/"+r.tag+"/"+archiveName, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(archive); err != nil {
			t.Errorf("failed to write archive response: %v", err)
		}
	})
	if !r.noChecksum {
		mux.HandleFunc("GET /releases/"+r.tag+"/"+archiveName+".sha256", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprintln(w, checksumBody)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, digest
}

// makeArchive builds an in-memory release archive (tar.gz, or zip when the
// platform's release format is zip) containing a single executable file.
func makeArchive(t *testing.T, isZip bool, entryName string, contents []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if isZip {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(entryName)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write(contents); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close zip: %v", err)
		}
		return buf.Bytes()
	}

	gw := gzip.NewWriter(&buf)
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
	return buf.Bytes()
}

// fakeInstalledBinary creates a stand-in for the currently running binary in
// its own temp dir (never the real test binary) and returns its path.
func fakeInstalledBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), binaryFilename())
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write fake installed binary: %v", err)
	}
	return path
}

// withCurrentBinary makes the upgrade treat path as the currently running
// binary instead of the real test binary. The override is installed when the
// test case runs (runOptions are applied per subtest) and restored by t's
// cleanups.
func withCurrentBinary(t *testing.T, path string) runOption {
	return func(*runConfig) {
		original := resolveCurrentBinaryPath
		resolveCurrentBinaryPath = func() (string, error) { return path, nil }
		t.Cleanup(func() { resolveCurrentBinaryPath = original })
	}
}

func TestUpgradeCmd(t *testing.T) {
	archiveName, _, err := buildReleaseArchiveName()
	if err != nil {
		t.Skipf("unsupported platform for release archives: %v", err)
	}

	latestServer := startFakeReleasesServer(t, "v99.99.99")

	newBinary := []byte("#!/bin/sh\necho fake tiger\n")
	release := fakeRelease{
		latest:    "v99.99.99",
		tag:       "v1.2.3",
		entryName: binaryFilename(),
		contents:  newBinary,
	}

	successServer, _ := startReleaseServer(t, release)

	mismatchRelease := release
	mismatchRelease.checksum = "deadbeef"
	mismatchServer, mismatchDigest := startReleaseServer(t, mismatchRelease)

	noChecksumRelease := release
	noChecksumRelease.noChecksum = true
	noChecksumServer, _ := startReleaseServer(t, noChecksumRelease)

	wrongEntryRelease := release
	wrongEntryRelease.entryName = "not-" + binaryFilename()
	wrongEntryServer, _ := startReleaseServer(t, wrongEntryRelease)

	// Stdout the command prints before each download-flow failure or success:
	// the version banner and the download line.
	downloadHeader := func(verb, currentVersion, serverURL string) string {
		return fmt.Sprintf("%s tiger %s → v1.2.3\nDownloading %s/releases/v1.2.3/%s\n",
			verb, currentVersion, serverURL, archiveName)
	}

	successBin := fakeInstalledBinary(t)
	downgradeBin := fakeInstalledBinary(t)
	archive404Bin := fakeInstalledBinary(t)
	checksum404Bin := fakeInstalledBinary(t)
	mismatchBin := fakeInstalledBinary(t)
	wrongEntryBin := fakeInstalledBinary(t)

	tests := []cmdTest{
		{
			name:    "rejects invalid --version",
			args:    []string{"upgrade", "--version", "not-a-version"},
			opts:    []runOption{withConfig(map[string]any{"releases_url": latestServer.URL})},
			wantErr: `invalid version "not-a-version": must be a valid semver version (e.g. v1.2.3)`,
		},
		{
			name:    "update alias rejects invalid --version",
			args:    []string{"update", "--version", "nope"},
			opts:    []runOption{withConfig(map[string]any{"releases_url": latestServer.URL})},
			wantErr: `invalid version "nope": must be a valid semver version (e.g. v1.2.3)`,
		},
		{
			// config.Version is "dev" in tests, so every invocation without
			// --force exercises the dev-build guard.
			name:    "refuses dev build without --force",
			args:    []string{"upgrade"},
			opts:    []runOption{withConfig(map[string]any{"releases_url": latestServer.URL})},
			wantErr: "tiger is a local dev build, not a released version; re-run with --force to replace it with version v99.99.99",
		},
		{
			name: "fails when release archive is missing",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": latestServer.URL}),
				withCurrentBinary(t, archive404Bin),
			},
			wantStdout: downloadHeader("Upgrading", "dev", latestServer.URL),
			wantErr: fmt.Sprintf("failed to download release archive: unexpected status code 404 for %s/releases/v1.2.3/%s",
				latestServer.URL, archiveName),
		},
		{
			name: "fails when checksum file is missing",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": noChecksumServer.URL}),
				withCurrentBinary(t, checksum404Bin),
			},
			wantStdout: downloadHeader("Upgrading", "dev", noChecksumServer.URL) + "Verifying checksum\n",
			wantErr: fmt.Sprintf("failed to fetch checksum: unexpected status code 404 for %s/releases/v1.2.3/%s.sha256",
				noChecksumServer.URL, archiveName),
		},
		{
			name: "fails on checksum mismatch",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": mismatchServer.URL}),
				withCurrentBinary(t, mismatchBin),
			},
			wantStdout: downloadHeader("Upgrading", "dev", mismatchServer.URL) + "Verifying checksum\n",
			wantErr:    fmt.Sprintf("checksum mismatch for %s: expected deadbeef, got %s", archiveName, mismatchDigest),
		},
		{
			name: "fails when binary is missing from archive",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": wrongEntryServer.URL}),
				withCurrentBinary(t, wrongEntryBin),
			},
			wantStdout: downloadHeader("Upgrading", "dev", wrongEntryServer.URL) + "Verifying checksum\n",
			wantErr:    fmt.Sprintf("failed to extract archive: binary %q not found in archive", binaryFilename()),
		},
		{
			// The un-prefixed --version also proves the release tag gets its
			// leading "v" normalized into the download path.
			name: "replaces the installed binary",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": successServer.URL}),
				withCurrentBinary(t, successBin),
			},
			wantStdout: downloadHeader("Upgrading", "dev", successServer.URL) +
				"Verifying checksum\n" +
				fmt.Sprintf("Installing new binary to %s\n", successBin) +
				"tiger upgraded successfully to v1.2.3\n",
			check: func(t *testing.T, result cmdResult) {
				got, err := os.ReadFile(successBin)
				if err != nil {
					t.Fatalf("read replaced binary: %v", err)
				}
				if !bytes.Equal(got, newBinary) {
					t.Errorf("replaced contents = %q, want %q", got, newBinary)
				}
				info, err := os.Stat(successBin)
				if err != nil {
					t.Fatalf("stat replaced binary: %v", err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
					t.Errorf("replaced binary is not executable: mode %v", info.Mode())
				}
				// No write-check probes or staged binaries may be left behind.
				leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(successBin), ".tiger-upgrade-*"))
				if len(leftovers) != 0 {
					t.Errorf("temp files left in install dir: %v", leftovers)
				}
			},
		},
		{
			name: "downgrades when requested version is older",
			args: []string{"upgrade", "--force", "--version", "1.2.3"},
			opts: []runOption{
				withConfig(map[string]any{"releases_url": successServer.URL}),
				withCurrentBinary(t, downgradeBin),
				withCLIVersion(t, "2.0.0"),
			},
			wantStdout: downloadHeader("Downgrading", "2.0.0", successServer.URL) +
				"Verifying checksum\n" +
				fmt.Sprintf("Installing new binary to %s\n", downgradeBin) +
				"tiger downgraded successfully to v1.2.3\n",
		},
	}
	runCmdTests(t, tests)

	// Error from a network failure is non-deterministic (depends on the net
	// stack's exact wording), so we assert only the stable wrapping prefix.
	t.Run("fails when latest version cannot be fetched", func(t *testing.T) {
		result := runCommand(t, []string{"upgrade"}, nil,
			withConfig(map[string]any{"releases_url": "http://127.0.0.1:1"}))
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantPrefix = "failed to check for latest version: "
		if !strings.HasPrefix(result.err.Error(), wantPrefix) {
			t.Errorf("unexpected error: %v (want prefix %q)", result.err, wantPrefix)
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

// isDowngrade's boundary matrix (equal versions, current newer/older, dev and
// unknown builds) isn't fully reachable through the command — the verb it picks
// is only observable on paths that need a released current version — so it
// keeps a helper-level table.
func TestIsDowngrade(t *testing.T) {
	cases := []struct {
		current, target string
		want            bool
	}{
		{"0.20.5", "v0.20.4", true},
		{"0.20.4", "v0.20.5", false},
		{"0.20.5", "v0.20.5", false},
		{"dev", "v0.20.5", false},
		{"unknown", "v0.20.5", false},
	}
	for _, tc := range cases {
		if got := isDowngrade(tc.current, tc.target); got != tc.want {
			t.Errorf("isDowngrade(%q, %q) = %v, want %v", tc.current, tc.target, got, tc.want)
		}
	}
}

// The archive-name matrix is platform-dependent (runtime.GOOS/GOARCH can't be
// changed in a test), so it keeps a helper-level test for the current platform.
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

// Platform-dependent like the archive name, so also kept at helper level.
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
