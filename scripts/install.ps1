# Tiger CLI Installation Script for Windows
#
# This script automatically downloads and installs the latest version of Tiger
# CLI from the release server. It downloads the appropriate binary for Windows
# x86_64 systems.
#
# Usage:
#   iwr -useb https://cli.tigerdata.com/install.ps1 | iex
#
# Environment Variables (all optional):
#   $env:VERSION           - Specific version to install (e.g., "v1.2.3")
#                            Default: installs the latest version
#
#   $env:INSTALL_DIR       - Custom installation directory
#                            Default: auto-detects best location
#
# Requirements:
#   - PowerShell 5.1 or higher
#   - Internet connection for downloading

$ErrorActionPreference = "Stop"

# Configuration
$RepoName = "tiger-cli"
$BinaryName = "tiger.exe"
$DownloadBaseUrl = "https://cli.tigerdata.com"

# Logging functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# Get version (from VERSION env var or latest from CloudFront)
function Get-Version {
    if ($env:VERSION) {
        Write-Info "Using specified version: $env:VERSION"
        return $env:VERSION
    }

    $url = "$DownloadBaseUrl/latest.txt"

    # Try to get version from latest.txt file with retry logic
    $maxRetries = 3
    $retryCount = 0
    $backoffSeconds = 1

    while ($retryCount -le $maxRetries) {
        try {
            Write-Info "Fetching latest version..."
            $version = (Invoke-WebRequest -Uri $url -UseBasicParsing).Content.Trim()

            if ([string]::IsNullOrWhiteSpace($version)) {
                throw "latest.txt file is empty"
            }

            Write-Info "Latest version: $version"
            return $version
        }
        catch {
            $retryCount++
            if ($retryCount -le $maxRetries) {
                Write-Warn "Latest version fetch failed, retrying ($retryCount/$maxRetries)..."
                Start-Sleep -Seconds $backoffSeconds
                $backoffSeconds *= 2
            }
            else {
                Write-ErrorMsg "Failed to fetch latest version after $($maxRetries + 1) attempts"
                Write-ErrorMsg "URL: $url"
                Write-ErrorMsg "Error: $_"
                exit 1
            }
        }
    }
}

# Check if a directory is in PATH
function Test-InPath {
    param([string]$Directory)

    $pathDirs = $env:PATH -split ';'
    foreach ($dir in $pathDirs) {
        if ($dir.TrimEnd('\') -eq $Directory.TrimEnd('\')) {
            return $true
        }
    }
    return $false
}

# Ensure a directory exists and is writable
function Test-WritableDir {
    param([string]$Directory)

    if (Test-Path $Directory) {
        # Check if writable by attempting to create a test file
        $testFile = Join-Path $Directory ".tiger-install-test"
        try {
            [IO.File]::OpenWrite($testFile).Close()
            Remove-Item $testFile -ErrorAction SilentlyContinue
            return $true
        }
        catch {
            return $false
        }
    }
    else {
        # Try to create the directory
        try {
            New-Item -ItemType Directory -Path $Directory -Force | Out-Null
            return $true
        }
        catch {
            return $false
        }
    }
}

# Find the best install directory
function Get-InstallDir {
    # If user specified INSTALL_DIR, respect it and try to use it
    if ($env:INSTALL_DIR) {
        if (Test-WritableDir $env:INSTALL_DIR) {
            Write-Info "Using user-specified install directory: $env:INSTALL_DIR"
            return $env:INSTALL_DIR
        }
        else {
            Write-ErrorMsg "User-specified install directory is not writable: $env:INSTALL_DIR"
            exit 1
        }
    }

    $candidateDirs = @(
        "$env:LOCALAPPDATA\Programs\TigerCLI",
        "$env:USERPROFILE\.local\bin",
        "$env:ProgramFiles\TigerCLI"
    )

    # Priority 1: Try to find a directory that's writable and in PATH
    foreach ($dir in $candidateDirs) {
        if ((Test-WritableDir $dir) -and (Test-InPath $dir)) {
            Write-Info "Selected install directory: $dir"
            return $dir
        }
    }

    # Priority 2: Try to find any directory that's writable (not in PATH)
    foreach ($dir in $candidateDirs) {
        if (Test-WritableDir $dir) {
            Write-Info "Selected install directory: $dir"
            return $dir
        }
    }

    # No suitable directory found, fail with clear error
    Write-ErrorMsg "Cannot find a writable install directory"
    Write-ErrorMsg "Tried the following directories: $($candidateDirs -join ', ')"
    Write-ErrorMsg "Please set `$env:INSTALL_DIR environment variable to a writable directory"
    exit 1
}

# Build archive name for Windows
function Get-ArchiveName {
    return "${RepoName}_Windows_x86_64.zip"
}

# Download file with retry logic
function Get-FileWithRetry {
    param(
        [string]$Url,
        [string]$OutputFile,
        [string]$Description
    )

    $maxRetries = 3
    $retryCount = 0
    $backoffSeconds = 1

    Write-Info "Downloading $Description..."
    Write-Info "URL: $Url"

    while ($retryCount -le $maxRetries) {
        try {
            Invoke-WebRequest -Uri $Url -OutFile $OutputFile -UseBasicParsing
            return
        }
        catch {
            $retryCount++
            if ($retryCount -le $maxRetries) {
                Write-Warn "$Description download failed, retrying ($retryCount/$maxRetries)..."
                Start-Sleep -Seconds $backoffSeconds
                $backoffSeconds *= 2
            }
            else {
                Write-ErrorMsg "Failed to download $Description after $($maxRetries + 1) attempts"
                Write-ErrorMsg "URL: $Url"
                Write-ErrorMsg "Error: $_"
                exit 1
            }
        }
    }
}

# Verify checksum
function Test-Checksum {
    param(
        [string]$Version,
        [string]$Filename,
        [string]$TmpDir
    )

    # Construct individual checksum file URL
    $checksumUrl = "$DownloadBaseUrl/releases/$Version/${Filename}.sha256"
    $checksumFile = Join-Path $TmpDir "${Filename}.sha256"

    # Download checksum file with retry logic
    Get-FileWithRetry -Url $checksumUrl -OutputFile $checksumFile -Description "checksum file"

    Write-Info "Validating checksum for $Filename..."

    # Read expected checksum
    $expectedChecksum = (Get-Content $checksumFile).Trim()

    # Calculate actual checksum
    $actualChecksum = (Get-FileHash -Path (Join-Path $TmpDir $Filename) -Algorithm SHA256).Hash.ToLower()

    if ($actualChecksum -ne $expectedChecksum) {
        Write-ErrorMsg "Checksum validation failed"
        Write-ErrorMsg "Expected: $expectedChecksum"
        Write-ErrorMsg "Actual: $actualChecksum"
        Write-ErrorMsg "For security reasons, installation has been aborted"
        exit 1
    }
}

# Download and verify archive
function Get-Archive {
    param(
        [string]$Version,
        [string]$ArchiveName,
        [string]$TmpDir
    )

    # Construct download URL
    $downloadUrl = "$DownloadBaseUrl/releases/$Version/$ArchiveName"

    # Download archive with retry logic
    Get-FileWithRetry -Url $downloadUrl -OutputFile (Join-Path $TmpDir $ArchiveName) -Description "Tiger CLI $Version for Windows x86_64"

    # Download and validate checksum
    Write-Info "Verifying file integrity..."
    Test-Checksum -Version $Version -Filename $ArchiveName -TmpDir $TmpDir
}

# Extract archive and return path to binary
function Expand-Archive {
    param(
        [string]$ArchiveName,
        [string]$TmpDir
    )

    Write-Info "Extracting archive..."

    $archivePath = Join-Path $TmpDir $ArchiveName
    Expand-Archive -Path $archivePath -DestinationPath $TmpDir -Force

    $binaryPath = Join-Path $TmpDir $BinaryName

    # Verify binary exists
    if (-not (Test-Path $binaryPath)) {
        Write-ErrorMsg "Binary not found in archive"
        exit 1
    }

    return $binaryPath
}

# Verify installation
function Test-Installation {
    param([string]$InstallDir)

    $binaryPath = Join-Path $InstallDir $BinaryName

    # Check if binary exists at expected location
    if (-not (Test-Path $binaryPath)) {
        Write-ErrorMsg "Installation verification failed: Binary not found at $binaryPath"
        exit 1
    }

    # Test that the binary is executable and get version
    try {
        $installedVersion = & $binaryPath version -o bare --skip-update-check 2>$null | Select-Object -First 1
        if ($installedVersion) {
            Write-Success "Tiger CLI installed successfully!"
            Write-Success "Version: $installedVersion"
        }
        else {
            Write-Success "Binary installed successfully at $binaryPath"
        }
    }
    catch {
        Write-ErrorMsg "Installation verification failed: Binary exists but is not executable"
        Write-ErrorMsg "Error: $_"
        exit 1
    }

    # Check if install directory is in PATH
    if (-not (Test-InPath $InstallDir)) {
        Write-Warn "Warning: $InstallDir is not in your PATH"
        Write-Warn "Add this directory to your PATH environment variable:"
        Write-Warn "  [System.Environment]::SetEnvironmentVariable('PATH', `"`$env:PATH;$InstallDir`", 'User')"
        Write-Warn ""
        Write-Warn "Or run the binary directly: $binaryPath"
    }
}

# Main installation process
function Install-TigerCLI {
    Write-Info "Tiger CLI Installation Script"
    Write-Info "=============================="

    # Get version (handles VERSION env var internally)
    $version = Get-Version

    # Find and ensure install directory exists and get its path
    $installDir = Get-InstallDir

    # Create temporary directory
    $tmpDir = Join-Path $env:TEMP "tiger-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Build archive name for Windows
        $archiveName = Get-ArchiveName

        # Download and verify the archive
        Get-Archive -Version $version -ArchiveName $archiveName -TmpDir $tmpDir

        # Extract the archive and get binary path
        $binaryPath = Expand-Archive -ArchiveName $archiveName -TmpDir $tmpDir

        # Copy binary to install directory
        # Remove existing binary first to prevent errors related
        # to swapping out a currently executing binary
        Write-Info "Installing to $installDir..."
        $targetPath = Join-Path $installDir $BinaryName
        if (Test-Path $targetPath) {
            Remove-Item $targetPath -Force
        }
        Copy-Item $binaryPath $targetPath -Force

        # Verify installation
        Test-Installation -InstallDir $installDir

        # Show usage information
        Write-Success "Get started with:"
        Write-Success "    tiger auth login"
        Write-Success "    tiger mcp install"
        Write-Success "For help:"
        Write-Success "    tiger help"
        Write-Success "Happy coding with Tiger CLI! 🐅"
    }
    finally {
        # Clean up temporary directory
        if (Test-Path $tmpDir) {
            Remove-Item $tmpDir -Recurse -Force
        }
    }
}

# Run main function
Install-TigerCLI
