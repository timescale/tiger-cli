#!/bin/sh
# shellcheck shell=dash
# shellcheck disable=SC2039 # local is non-POSIX
#
# Tiger CLI Installation Script
#
# This script automatically downloads and installs the latest version of Tiger CLI
# from S3 releases. It detects your platform (OS and architecture) and downloads
# the appropriate binary for your system.
#
# Usage:
#   curl -fsSL https://tiger-cli-releases.s3.amazonaws.com/install/install.sh | sh
#
# Environment Variables (all optional):
#   VERSION           - Specific version to install (e.g., "v1.2.3")
#                       Default: installs the latest version
#
#   INSTALL_DIR       - Custom installation directory
#                       Default: auto-detects best location
#
# Supported Platforms:
#   - Linux (x86_64, i386, arm64, armv7)
#   - macOS/Darwin (x86_64, arm64)
#   - Windows (x86_64)
#
# Requirements:
#   - curl (for downloading)
#   - tar/unzip (for extracting archives)
#   - shasum/sha256sum (for verifying checksums)
#   - Standard POSIX utilities (mktemp, chmod, etc.)
set -eu

# Configuration
REPO_NAME="tiger-cli"
BINARY_NAME="tiger"

# S3 Configuration (primary download source)
S3_BUCKET="tiger-cli-releases"
S3_BASE_URL="https://${S3_BUCKET}.s3.amazonaws.com"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    printf "%b[INFO]%b %s\n" "${BLUE}" "${NC}" "$1" >&2
}

log_success() {
    printf "%b[SUCCESS]%b %s\n" "${GREEN}" "${NC}" "$1" >&2
}

log_warn() {
    printf "%b[WARN]%b %s\n" "${YELLOW}" "${NC}" "$1" >&2
}

log_error() {
    printf "%b[ERROR]%b %s\n" "${RED}" "${NC}" "$1" >&2
}

# Detect OS and architecture
detect_platform() {
    local os
    local arch

    # Detect OS
    case "$(uname -s)" in
        Darwin*) os="darwin" ;;
        Linux*)  os="linux" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *) log_error "Unsupported operating system: $(uname -s)"; exit 1 ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64) arch="x86_64" ;;
        i386|i686) arch="i386" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l) arch="armv7" ;;
        *) log_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

# Verify that all required dependencies are available
verify_dependencies() {
    local platform="$1"

    # Build complete dependency list based on platform
    local required_deps="curl mktemp head tr sed awk grep uname chmod cp mkdir sleep"

    if echo "${platform}" | grep -q "windows"; then
        required_deps="${required_deps} unzip"
    else
        required_deps="${required_deps} tar"
    fi

    # Check if all commands are available
    local missing_deps=""
    local cmd

    for cmd in ${required_deps}; do
        if ! command -v "${cmd}" >/dev/null 2>&1; then
            missing_deps="${missing_deps} ${cmd}"
        fi
    done

    if [ -n "${missing_deps}" ]; then
        log_error "Missing required dependencies:${missing_deps}"
        log_error "Please install these tools and try again"
        exit 1
    fi
}

# Download a URL to stdout with retry logic
fetch_with_retry() {
    local url="$1"
    local description="${2:-content}"
    local max_retries=3
    local retry_count=0
    local backoff_seconds=1

    while [ ${retry_count} -le "${max_retries}" ]; do
        local content
        if content=$(curl -fsSL "${url}" 2>/dev/null); then
            echo "${content}"
            return 0
        else
            retry_count=$((retry_count + 1))
            if [ "${retry_count}" -le "${max_retries}" ]; then
                log_warn "${description} fetch failed, retrying (${retry_count}/${max_retries})..."
                sleep ${backoff_seconds}
                backoff_seconds=$((backoff_seconds * 2))
            else
                log_error "Failed to fetch ${description} after $((max_retries + 1)) attempts"
                log_error "URL: ${url}"
                exit 1
            fi
        fi
    done
}

# Download a file with retry logic
download_with_retry() {
    local url="$1"
    local output_file="$2"
    local description="${3:-file}"
    local max_retries=3
    local retry_count=0
    local backoff_seconds=1

    log_info "Downloading ${description}..."
    log_info "URL: ${url}"

    while [ ${retry_count} -le "${max_retries}" ]; do
        if curl -fsSL "${url}" -o "${output_file}"; then
            return 0
        else
            retry_count=$((retry_count + 1))
            if [ "${retry_count}" -le "${max_retries}" ]; then
                log_warn "${description} download failed, retrying (${retry_count}/${max_retries})..."
                sleep ${backoff_seconds}
                backoff_seconds=$((backoff_seconds * 2))
            else
                log_error "Failed to download ${description} after $((max_retries + 1)) attempts"
                log_error "URL: ${url}"
                exit 1
            fi
        fi
    done
}

# Get version (from VERSION env var or latest from S3)
get_version() {
    # Use VERSION env var if provided
    if [ -n "${VERSION:-}" ]; then
        log_info "Using specified version: ${VERSION}"
        echo "${VERSION}"
        return
    fi

    local url="${S3_BASE_URL}/install/latest.txt"

    # Try to get version from S3 latest.txt file at bucket root
    local version
    version=$(fetch_with_retry "${url}" "latest version")

    # Clean up the version string
    version=$(echo "${version}" | head -n1 | tr -d '\n\r')

    if [ -z "${version}" ]; then
        log_error "latest.txt file is empty"
        exit 1
    fi

    log_info "Latest version: ${version}"
    echo "${version}"
}

# Check if a directory is in PATH
is_in_path() {
    local dir="$1"
    case ":${PATH}:" in
        *":${dir}:"*) return 0 ;;
        *) return 1 ;;
    esac
}

# Ensure a directory exists and is writable, creating it if needed
ensure_writable_dir() {
    local dir="$1"

    if [ -d "${dir}" ] && [ -w "${dir}" ]; then
        return 0  # Directory exists and is writable
    elif [ ! -e "${dir}" ] && [ -w "$(dirname "${dir}")" ]; then
        # Directory doesn't exist but parent is writable - create it
        mkdir -p "${dir}"
        return 0
    else
        return 1  # Neither condition met
    fi
}

# Find the best install directory and ensure it exists
detect_install_dir() {
    # If user specified INSTALL_DIR, respect it and try to use it
    if [ -n "${INSTALL_DIR:-}" ]; then
        if ensure_writable_dir "${INSTALL_DIR}"; then
            log_info "Using user-specified install directory: ${INSTALL_DIR}"
            echo "${INSTALL_DIR}"
            return
        else
            log_error "User-specified install directory is not writable: ${INSTALL_DIR}"
            exit 1
        fi
    fi

    local candidate_dirs="$HOME/.local/bin $HOME/bin"

    # Priority 1: Try to find a directory that's writable/creatable and in PATH
    for dir in ${candidate_dirs}; do
        if ensure_writable_dir "${dir}" && is_in_path "${dir}"; then
            log_info "Selected install directory: ${dir}"
            echo "${dir}"
            return
        fi
    done

    # Priority 2: Try to find any directory that's writable/creatable (not in PATH)
    for dir in ${candidate_dirs}; do
        if ensure_writable_dir "${dir}"; then
            log_info "Selected install directory: ${dir}"
            echo "${dir}"
            return
        fi
    done

    # No suitable directory found, fail with clear error
    log_error "Cannot find a writable install directory"
    log_error "Tried the following directories: ${candidate_dirs}"
    log_error "Please set INSTALL_DIR environment variable to a writable directory"
    exit 1
}


# Build archive name based on platform
build_archive_name() {
    local platform="$1"

    if [ "${platform}" = "windows_x86_64" ]; then
        echo "${REPO_NAME}_Windows_x86_64.zip"
    else
        echo "${REPO_NAME}_$(echo "${platform}" | sed 's/_/ /' | awk '{print toupper(substr($1,1,1)) tolower(substr($1,2)) "_" $2}').tar.gz"
    fi
}

# Download and validate checksum file
verify_checksum() {
    local version="$1"
    local filename="$2"
    local tmp_dir="$3"

    # Construct individual checksum file URL
    local checksum_url="${S3_BASE_URL}/releases/${version}/${filename}.sha256"
    local checksum_file="${tmp_dir}/${filename}.sha256"

    # Download checksum file with retry logic
    download_with_retry "${checksum_url}" "${checksum_file}" "checksum file"

    log_info "Validating checksum for ${filename}..."

    cd "${tmp_dir}"

    # Format checksum for validation: "hash  filename"
    local formatted_checksum
    formatted_checksum=$(printf "%s  %s\n" "$(cat "${checksum_file}" | tr -d '[:space:]')" "${filename}")

    if command -v sha256sum >/dev/null 2>&1; then
        if ! echo "${formatted_checksum}" | sha256sum -c >/dev/null 2>&1; then
            log_error "Checksum validation failed using sha256sum"
            log_error "For security reasons, installation has been aborted"
            exit 1
        fi
    elif command -v shasum >/dev/null 2>&1; then
        if ! echo "${formatted_checksum}" | shasum -a 256 -c >/dev/null 2>&1; then
            log_error "Checksum validation failed using shasum"
            log_error "For security reasons, installation has been aborted"
            exit 1
        fi
    else
        log_error "No SHA256 utility available (tried sha256sum, shasum)"
        log_error "Checksum validation is required for security"
        log_error "Please install sha256sum or shasum and try again"
        exit 1
    fi
}

# Download archive and verify checksum
download_archive() {
    local version="$1"
    local archive_name="$2"
    local tmp_dir="$3"
    local platform="$4"

    # Construct S3 download URL
    local download_url="${S3_BASE_URL}/releases/${version}/${archive_name}"

    # Download archive with retry logic
    download_with_retry "${download_url}" "${tmp_dir}/${archive_name}" "Tiger CLI ${version} for ${platform}"

    # Download and validate checksum
    log_info "Verifying file integrity..."
    verify_checksum "${version}" "${archive_name}" "${tmp_dir}"
}

# Extract archive and return path to binary
extract_archive() {
    local archive_name="$1"
    local tmp_dir="$2"
    local platform="$3"

    log_info "Extracting archive..."
    cd "${tmp_dir}"

    local binary_path
    if [ "${platform}" = "windows_x86_64" ]; then
        unzip -q "${archive_name}"
        binary_path="${tmp_dir}/${BINARY_NAME}.exe"
    else
        tar -xzf "${archive_name}"
        binary_path="${tmp_dir}/${BINARY_NAME}"
    fi

    # Verify binary exists
    if [ ! -f "${binary_path}" ]; then
        log_error "Binary not found in archive"
        exit 1
    fi

    # Make binary executable
    chmod +x "${binary_path}"

    echo "${binary_path}"
}

# Verify installation
verify_installation() {
    local install_dir="$1"
    local binary_path="${install_dir}/${BINARY_NAME}"

    # First, check if binary exists at expected location
    if [ ! -f "${binary_path}" ]; then
        log_error "Installation verification failed: Binary not found at ${binary_path}"
        exit 1
    fi

    # Test that the binary is executable and get version
    local installed_version
    if installed_version=$("${binary_path}" version 2>/dev/null | head -n1 || echo ""); then
        if [ -n "${installed_version}" ]; then
            log_success "Tiger CLI installed successfully!"
            log_success "Version: ${installed_version}"
        else
            log_success "Binary installed successfully at ${binary_path}"
        fi
    else
        log_error "Installation verification failed: Binary exists but is not executable"
        exit 1
    fi

    # Check if install directory is in PATH
    if ! is_in_path "${install_dir}"; then
        log_warn "Warning: ${install_dir} is not in your PATH"
        log_warn "Add this to your shell profile (.bashrc, .zshrc, etc.):"
        log_warn "    export PATH=\"${install_dir}:\${PATH}\""
        log_warn ""
        log_warn "Or run the binary directly: ${binary_path}"
    fi
}

# Main installation process
main() {
    log_info "Tiger CLI Installation Script"
    log_info "=============================="

    # Detect platform first (needed for dependency checking)
    local platform
    platform=$(detect_platform)
    log_info "Detected platform: ${platform}"

    # Verify all required dependencies are available
    verify_dependencies "${platform}"

    # Get version (handles VERSION env var internally)
    local version
    version="$(get_version)"

    # Find and ensure install directory exists and get its path
    local install_dir
    install_dir="$(detect_install_dir)"

    # Create temporary directory
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    # shellcheck disable=SC2064 # We want to expand ${tmp_dir} immediately, because it's out-of-scope when EXIT fires
    trap "rm -rf '${tmp_dir}'" EXIT

    # Build archive name for the platform
    local archive_name
    archive_name="$(build_archive_name "${platform}")"

    # Download and verify the archive
    download_archive "${version}" "${archive_name}" "${tmp_dir}" "${platform}"

    # Extract the archive and get binary path
    local binary_path
    binary_path="$(extract_archive "${archive_name}" "${tmp_dir}" "${platform}")"

    # Copy binary to install directory
    log_info "Installing to ${install_dir}..."
    cp "${binary_path}" "${install_dir}/${BINARY_NAME}"

    # Verify installation
    verify_installation "${install_dir}"

    # Show usage information
    log_success "Get started with:"
    log_success "    ${BINARY_NAME} auth login"
    log_success "For help:"
    log_success "    ${BINARY_NAME} --help"
    log_success "Happy coding with Tiger CLI! 🐅"
}

# Run main function
main "$@"
