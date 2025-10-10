package version

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// InstallMethod represents how Tiger CLI was installed
type InstallMethod string

const (
	InstallMethodHomebrew    InstallMethod = "homebrew"
	InstallMethodDeb         InstallMethod = "deb"
	InstallMethodRPM         InstallMethod = "rpm"
	InstallMethodInstallSh   InstallMethod = "install_sh"
	InstallMethodUnknown     InstallMethod = "unknown"
	InstallMethodDevelopment InstallMethod = "development"
)

// CheckResult contains the result of a version check
type CheckResult struct {
	UpdateAvailable bool
	LatestVersion   string
	CurrentVersion  string
	InstallMethod   InstallMethod
	UpdateCommand   string
}

// ShouldCheck returns true if enough time has passed since the last check
func shouldCheck(lastCheckTime int64, interval int) bool {
	if interval == 0 {
		return false // Version check disabled
	}

	if lastCheckTime == 0 {
		return true // Never checked before
	}

	lastCheck := time.Unix(lastCheckTime, 0)
	nextCheck := lastCheck.Add(time.Duration(interval) * time.Second)
	return time.Now().After(nextCheck)
}

// FetchLatestVersion downloads the latest version string from the given URL
func fetchLatestVersion(checkURL string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(checkURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Trim whitespace and leading "v"
	version := strings.TrimPrefix(strings.TrimSpace(string(body)), "v")
	if version == "" {
		return "", fmt.Errorf("empty version string in response")
	}

	return version, nil
}

// CompareVersions returns true if newVersion is greater than currentVersion
// This does a simple string comparison after removing the 'v' prefix
func compareVersions(currentVersion, newVersion string) bool {
	// Normalize versions by removing 'v' prefix
	current := strings.TrimPrefix(currentVersion, "v")
	latest := strings.TrimPrefix(newVersion, "v")

	// If versions are identical, no update available
	if current == latest {
		return false
	}

	// Handle development version
	if current == "dev" || current == "unknown" {
		return false
	}

	// For simplicity, do string comparison
	// This works for semantic versioning if we split and compare
	return latest > current
}

// DetectInstallMethod determines how Tiger CLI was installed
func detectInstallMethod(binaryPath string) InstallMethod {
	// Check for development build
	if strings.Contains(binaryPath, "go-build") {
		return InstallMethodDevelopment
	}

	// Check for Homebrew installation
	if strings.Contains(binaryPath, "/homebrew/") ||
		strings.Contains(binaryPath, "/Homebrew/") ||
		strings.Contains(binaryPath, "linuxbrew") {
		return InstallMethodHomebrew
	}

	// Try to detect via package managers
	absPath, err := filepath.Abs(binaryPath)
	if err == nil {
		// Check if installed via dpkg (Debian/Ubuntu)
		if runtime.GOOS == "linux" {
			if output, err := exec.Command("dpkg", "-S", absPath).CombinedOutput(); err == nil {
				if strings.Contains(string(output), "tiger-cli") {
					return InstallMethodDeb
				}
			}

			// Check if installed via rpm (RHEL/Fedora/CentOS)
			if output, err := exec.Command("rpm", "-qf", absPath).CombinedOutput(); err == nil {
				if strings.Contains(string(output), "tiger-cli") {
					return InstallMethodRPM
				}
			}
		}
	}

	// Check if installed via install.sh (typically in ~/.local/bin or ~/bin)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		localBin := filepath.Join(homeDir, ".local", "bin")
		homeBin := filepath.Join(homeDir, "bin")

		if strings.HasPrefix(binaryPath, localBin) || strings.HasPrefix(binaryPath, homeBin) {
			return InstallMethodInstallSh
		}
	}

	return InstallMethodUnknown
}

// GetUpdateCommand returns the command to update Tiger CLI based on the install method
func getUpdateCommand(method InstallMethod) string {
	switch method {
	case InstallMethodHomebrew:
		return "brew upgrade tiger-cli"
	case InstallMethodDeb:
		return "sudo apt update && sudo apt install tiger-cli"
	case InstallMethodRPM:
		// Try to detect which package manager is available
		if _, err := exec.LookPath("dnf"); err == nil {
			return "sudo dnf update tiger-cli"
		}
		return "sudo yum update tiger-cli"
	case InstallMethodInstallSh:
		return "curl -fsSL https://tiger-cli-releases.s3.amazonaws.com/install/install.sh | sh"
	case InstallMethodDevelopment:
		return "rebuild from source or install via package manager"
	default:
		return "visit https://github.com/timescale/tiger-cli/releases"
	}
}

func checkVersionForUpdate(version string, cfg *config.Config, output *io.Writer) (*CheckResult, error) {
	latestVersion, err := fetchLatestVersion(cfg.VersionCheckURL)
	if err != nil {
		return nil, err
	}

	updateAvailable := compareVersions(version, latestVersion)

	// Detect installation method
	binaryPath, err := os.Executable()
	if err != nil {
		if cfg.Debug && output != nil {
			fmt.Fprintf(*output, "Warning: failed to get executable path: %v\n", err)
		}
		binaryPath = ""
	}

	installMethod := detectInstallMethod(binaryPath)
	updateCommand := getUpdateCommand(installMethod)

	return &CheckResult{
		UpdateAvailable: updateAvailable,
		LatestVersion:   latestVersion,
		CurrentVersion:  version,
		InstallMethod:   installMethod,
		UpdateCommand:   updateCommand,
	}, nil
}

// CheckForUpdate checks if a new version is available and returns the result
func checkForUpdate(cfg *config.Config, output *io.Writer) (*CheckResult, error) {
	return checkVersionForUpdate(config.Version, cfg, output)
}

// PrintUpdateWarning prints a warning message to stderr if an update is available
func PrintUpdateWarning(result *CheckResult, cfg *config.Config, output *io.Writer) {
	if result == nil || output == nil {
		return
	}
	if !result.UpdateAvailable {
		if cfg.Debug {
			fmt.Fprintf(*output, "No update available\n")
		}
		return
	}

	fmt.Fprintf(*output, "\n\n%s %s → %s\nTo upgrade: %s\n",
		color.YellowString("A new release of tiger-cli is available:"),
		color.CyanString(result.CurrentVersion),
		color.CyanString(result.LatestVersion),
		result.UpdateCommand,
	)
}

// PerformCheck performs the full version check flow:
// 1. Check if enough time has passed
// 2. Fetch and compare versions
// 3. Update the last check timestamp
// 4. Print warning if update available
func PerformCheck(cfg *config.Config, output *io.Writer, force bool) *CheckResult {
	// Skip if version check is disabled
	if !force && cfg.VersionCheckInterval == 0 {
		if cfg.Debug && output != nil {
			fmt.Fprintln(*output, "Version check is disabled")
		}
		return nil
	}

	// Skip if not enough time has passed
	if !force && !shouldCheck(cfg.VersionCheckLastTime, cfg.VersionCheckInterval) {
		if cfg.Debug && output != nil {
			fmt.Fprintf(
				*output,
				"Skipping version check (too soon)\n  Last check: %d, Interval: %d seconds\n",
				cfg.VersionCheckLastTime,
				cfg.VersionCheckInterval,
			)
		}
		return nil
	}

	// Perform the check
	result, err := checkForUpdate(cfg, output)
	if err != nil {
		if cfg.Debug && output != nil {
			fmt.Fprintf(*output, "Warning: version check failed: %v\n", err)
		} else if output != nil {
			fmt.Fprintf(*output, "Warning: failed to check the latest CLI version.\n")
		}
		// Don't update last check time on error - we'll retry next time
		return nil
	}

	// Update last check time only after successful check
	// This ensures we retry quickly if the check failed
	now := time.Now().Unix()
	if err := cfg.Set("version_check_last_time", fmt.Sprintf("%d", now)); err != nil {
		if cfg.Debug && output != nil {
			fmt.Fprintf(*output, "Warning: failed to update last check time: %v\n", err)
		}
		// Don't fail if we can't update the timestamp
	}

	return result
}
