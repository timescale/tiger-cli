package version

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestShouldCheck(t *testing.T) {
	tests := []struct {
		name          string
		lastCheckTime int64
		interval      int
		want          bool
	}{
		{
			name:          "disabled when interval is 0",
			lastCheckTime: time.Now().Unix(),
			interval:      0,
			want:          false,
		},
		{
			name:          "never checked before",
			lastCheckTime: 0,
			interval:      3600,
			want:          true,
		},
		{
			name:          "checked recently",
			lastCheckTime: time.Now().Unix() - 1800, // 30 minutes ago
			interval:      3600,                     // 1 hour interval
			want:          false,
		},
		{
			name:          "checked long ago",
			lastCheckTime: time.Now().Unix() - 7200, // 2 hours ago
			interval:      3600,                     // 1 hour interval
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCheck(tt.lastCheckTime, tt.interval)
			if got != tt.want {
				t.Errorf("shouldCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		newVersion     string
		want           bool
	}{
		{
			name:           "same version",
			currentVersion: "v1.0.0",
			newVersion:     "v1.0.0",
			want:           false,
		},
		{
			name:           "same version without v prefix",
			currentVersion: "1.0.0",
			newVersion:     "1.0.0",
			want:           false,
		},
		{
			name:           "newer version available",
			currentVersion: "v1.0.0",
			newVersion:     "v1.1.0",
			want:           true,
		},
		{
			name:           "newer version available mixed prefix",
			currentVersion: "1.0.0",
			newVersion:     "v1.1.0",
			want:           true,
		},
		{
			name:           "older version (should not update)",
			currentVersion: "v1.1.0",
			newVersion:     "v1.0.0",
			want:           false,
		},
		{
			name:           "dev version (should not update)",
			currentVersion: "dev",
			newVersion:     "v1.0.0",
			want:           false,
		},
		{
			name:           "unknown version (should not update)",
			currentVersion: "unknown",
			newVersion:     "v1.0.0",
			want:           false,
		},
		{
			name:           "major version bump",
			currentVersion: "v1.9.9",
			newVersion:     "v2.0.0",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.currentVersion, tt.newVersion)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %v, want %v",
					tt.currentVersion, tt.newVersion, got, tt.want)
			}
		})
	}
}

func TestDetectInstallMethod(t *testing.T) {
	tests := []struct {
		name       string
		binaryPath string
		want       InstallMethod
	}{
		{
			name:       "homebrew on macOS",
			binaryPath: "/opt/homebrew/bin/tiger",
			want:       InstallMethodHomebrew,
		},
		{
			name:       "homebrew on linux",
			binaryPath: "/home/linuxbrew/.linuxbrew/bin/tiger",
			want:       InstallMethodHomebrew,
		},
		{
			name:       "development build",
			binaryPath: "/tmp/go-build123/exe/tiger",
			want:       InstallMethodDevelopment,
		},
		{
			name:       "install.sh in .local/bin",
			binaryPath: filepath.Join(os.Getenv("HOME"), ".local", "bin", "tiger"),
			want:       InstallMethodInstallSh,
		},
		{
			name:       "install.sh in ~/bin",
			binaryPath: filepath.Join(os.Getenv("HOME"), "bin", "tiger"),
			want:       InstallMethodInstallSh,
		},
		{
			name:       "unknown location",
			binaryPath: "/usr/local/bin/tiger",
			want:       InstallMethodUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectInstallMethod(tt.binaryPath)
			if got != tt.want {
				t.Errorf("detectInstallMethod(%q) = %v, want %v", tt.binaryPath, got, tt.want)
			}
		})
	}
}

func TestGetUpdateCommand(t *testing.T) {
	testUrl := "https://cli.example.com"
	tests := []struct {
		name   string
		method InstallMethod
		want   string
	}{
		{
			name:   "homebrew",
			method: InstallMethodHomebrew,
			want:   "brew upgrade tiger-cli",
		},
		{
			name:   "deb",
			method: InstallMethodDeb,
			want:   "sudo apt update && sudo apt install tiger-cli",
		},
		{
			name:   "rpm",
			method: InstallMethodRPM,
			want:   "sudo yum update tiger-cli", // or dnf depending on system
		},
		{
			name:   "install.sh",
			method: InstallMethodInstallSh,
			want:   "curl -fsSL " + testUrl + "/install/install.sh | sh",
		},
		{
			name:   "development",
			method: InstallMethodDevelopment,
			want:   "rebuild from source or install via package manager",
		},
		{
			name:   "unknown",
			method: InstallMethodUnknown,
			want:   "visit https://github.com/timescale/tiger-cli/releases",
		},
	}

	cfg := &config.Config{
		ReleasesURL: testUrl,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUpdateCommand(tt.method, cfg)
			// For RPM, accept either yum or dnf
			if tt.method == InstallMethodRPM {
				if got != "sudo yum update tiger-cli" && got != "sudo dnf update tiger-cli" {
					t.Errorf("getUpdateCommand(%v) = %v, want yum or dnf command", tt.method, got)
				}
			} else {
				if got != tt.want {
					t.Errorf("GetUpdateCommand(%v) = %v, want %v", tt.method, got, tt.want)
				}
			}
		})
	}
}

func TestFetchLatestVersion(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
		wantResult string
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			body:       "v1.2.3\n",
			wantErr:    false,
			wantResult: "1.2.3",
		},
		{
			name:       "success with whitespace",
			statusCode: http.StatusOK,
			body:       "  v1.2.3  \n",
			wantErr:    false,
			wantResult: "1.2.3",
		},
		{
			name:       "empty body",
			statusCode: http.StatusOK,
			body:       "",
			wantErr:    true,
		},
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			body:       "Not Found",
			wantErr:    true,
		},
		{
			name:       "500 server error",
			statusCode: http.StatusInternalServerError,
			body:       "Internal Server Error",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			// Test fetchLatestVersion
			got, err := fetchLatestVersion(server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantResult {
				t.Errorf("FetchLatestVersion() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestCheckForUpdate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("v1.5.0\n"))
	}))
	defer server.Close()

	tests := []struct {
		name           string
		currentVersion string
		wantUpdate     bool
		wantLatest     string
	}{
		{
			name:           "update available",
			currentVersion: "v1.0.0",
			wantUpdate:     true,
			wantLatest:     "1.5.0",
		},
		{
			name:           "already latest",
			currentVersion: "v1.5.0",
			wantUpdate:     false,
			wantLatest:     "1.5.0",
		},
		{
			name:           "development version",
			currentVersion: "dev",
			wantUpdate:     false,
			wantLatest:     "1.5.0",
		},
	}

	cfg := &config.Config{
		ReleasesURL: server.URL,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := checkVersionForUpdate(tt.currentVersion, cfg, nil)
			if err != nil {
				t.Errorf("checkVersionForUpdate() error = %v", err)
				return
			}
			if result.UpdateAvailable != tt.wantUpdate {
				t.Errorf("checkVersionForUpdate() UpdateAvailable = %v, want %v", result.UpdateAvailable, tt.wantUpdate)
			}
			if result.LatestVersion != tt.wantLatest {
				t.Errorf("checkVersionForUpdate() LatestVersion = %v, want %v", result.LatestVersion, tt.wantLatest)
			}
			if result.CurrentVersion != tt.currentVersion {
				t.Errorf("checkVersionForUpdate() CurrentVersion = %v, want %v", result.CurrentVersion, tt.currentVersion)
			}
		})
	}
}

func TestPerformCheck_Disabled(t *testing.T) {
	// Create config with version check disabled
	cfg := &config.Config{
		VersionCheckInterval: 0, // Disabled
		ReleasesURL:          "http://example.com",
	}

	// Should return immediately without error
	err := PerformCheck(cfg, nil, false)
	if err != nil {
		t.Errorf("PerformCheck() with disabled check returned error: %v", err)
	}
}

func TestPerformCheck_TooSoon(t *testing.T) {
	// Create config with recent check
	cfg := &config.Config{
		VersionCheckInterval: 3600,              // 1 hour
		VersionCheckLastTime: time.Now().Unix(), // Just checked
		ReleasesURL:          "http://example.com",
	}

	// Should return immediately without error (and without making HTTP request)
	err := PerformCheck(cfg, nil, false)
	if err != nil {
		t.Errorf("PerformCheck() with recent check returned error: %v", err)
	}
}
