#!/bin/sh
# shellcheck shell=dash
# shellcheck disable=SC2039 # local is non-POSIX

set -eu

# Configuration
REPO_NAME="tiger-cli"
BINARY_NAME="tiger"

# S3 Configuration (primary download source)
S3_BUCKET="${TIGER_S3_BUCKET:-tiger-cli-releases}"
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

# Check if a directory is in PATH
is_in_path() {
    local dir="$1"
    case ":${PATH}:" in
        *":${dir}:"*) return 0 ;;
        *) return 1 ;;
    esac
}

# Find the best install directory
detect_install_dir() {
    # If user specified TIGER_INSTALL_DIR, respect it
    if [ -n "${TIGER_INSTALL_DIR:-}" ]; then
        echo "${TIGER_INSTALL_DIR}"
        return
    fi

    # Try to find a directory that's writable and in PATH
    local candidate_dirs="$HOME/.local/bin $HOME/bin /usr/local/bin"

    for dir in ${candidate_dirs}; do
        # Check if we can write to it (either exists and writable, or parent is writable)
        if ([ -d "${dir}" ] && [ -w "${dir}" ]) || [ -w "$(dirname "${dir}")" ]; then
            if is_in_path "${dir}"; then
                echo "${dir}"
                return
            fi
        fi
    done

    # No writable directory in PATH found, default to ~/.local/bin
    echo "$HOME/.local/bin"
}

# Check if commands are available, exit with error if any are missing
command_exists() {
    local missing_deps=""
    local cmd

    for cmd in "$@"; do
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


# Get latest version from S3
get_latest_version() {
    local url="${S3_BASE_URL}/install/latest.txt"

    # Try to get version from S3 latest.txt file at bucket root
    local version
    version=$(curl -fsSL "${url}" 2>/dev/null | head -n1 | tr -d '\n\r' || echo "")

    if [ -z "${version}" ]; then
        log_error "latest.txt file not found in S3 bucket root"
        log_error "URL: ${url}"
        exit 1
    fi

    echo "${version}"
}

# Download and install binary
install_binary() {
    local version="$1"
    local platform="$2"

    # Detect the best install directory
    local install_dir
    install_dir="$(detect_install_dir)"
    log_info "Selected install directory: ${install_dir}"

    # Create temporary directory
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    # shellcheck disable=SC2064 # We want to expand ${tmp_dir} immediately, because it's out-of-scope when EXIT fires
    trap "rm -rf '${tmp_dir}'" EXIT

    # Construct archive name
    local archive_name
    if [ "${platform}" = "windows_x86_64" ]; then
        archive_name="${REPO_NAME}_Windows_x86_64.zip"
    else
        archive_name="${REPO_NAME}_$(echo "${platform}" | sed 's/_/ /' | awk '{print toupper(substr($1,1,1)) tolower(substr($1,2)) "_" $2}').tar.gz"
    fi

    # Construct S3 download URL (artifacts are stored in releases/version/ directory)
    local download_url="${S3_BASE_URL}/releases/${version}/${archive_name}"

    log_info "Downloading Tiger CLI ${version} for ${platform}..."
    log_info "URL: ${download_url}"

    # Download archive with retry logic
    local max_retries=3
    local retry_count=0

    while [ ${retry_count} -lt ${max_retries} ]; do
        if curl -fsSL "${download_url}" -o "${tmp_dir}/${archive_name}"; then
            break
        else
            retry_count=$((retry_count + 1))
            if [ "${retry_count}" -lt "${max_retries}" ]; then
                log_warn "Download failed, retrying (${retry_count}/${max_retries})..."
                sleep 2
            else
                log_error "Failed to download Tiger CLI from S3 after ${max_retries} attempts"
                log_error "URL: ${download_url}"
                log_error "Please check that the S3 bucket contains the release files"
                exit 1
            fi
        fi
    done

    # Extract archive
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

    # Install binary
    log_info "Installing to ${install_dir}..."

    # Create install directory if it doesn't exist
    if [ ! -d "${install_dir}" ]; then
        if [ "${install_dir}" = "/usr/local/bin" ]; then
            sudo mkdir -p "${install_dir}"
        else
            mkdir -p "${install_dir}"
        fi
    fi

    # Copy binary
    if [ -w "${install_dir}" ]; then
        cp "${binary_path}" "${install_dir}/${BINARY_NAME}"
    else
        sudo cp "${binary_path}" "${install_dir}/${BINARY_NAME}"
    fi

    log_success "Tiger CLI installed successfully!"
}

# Verify installation
verify_installation() {
    local install_dir="$1"

    if command -v "${BINARY_NAME}" >/dev/null 2>&1; then
        local installed_version
        installed_version=$(${BINARY_NAME} version 2>/dev/null | head -n1 || echo "unknown")
        log_success "Installation verified: ${installed_version}"

        # Check if install directory is in PATH
        if ! is_in_path "${install_dir}"; then
            log_warn "Warning: ${install_dir} is not in your PATH"
            log_info "Add this to your shell profile (.bashrc, .zshrc, etc.):"
            log_info "    export PATH=\"${install_dir}:\${PATH}\""
        fi
    else
        log_error "Installation verification failed"
        exit 1
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

    # Check dependencies based on platform
    local common_deps="curl mktemp head tr sed awk grep uname chmod cp mkdir sleep"

    if echo "${platform}" | grep -q "windows"; then
        # shellcheck disable=SC2086 # Word splitting intended for common_deps
        command_exists ${common_deps} unzip
    else
        # shellcheck disable=SC2086 # Word splitting intended for common_deps
        command_exists ${common_deps} tar
    fi

    # Get version (use VERSION env var if provided, otherwise get latest)
    local version
    if [ -n "${VERSION:-}" ]; then
        version="${VERSION}"
        log_info "Using specified version: ${version}"
    else
        version="$(get_latest_version)"
        log_info "Latest version: ${version}"
    fi

    # Install binary and get the install directory used
    local install_dir
    install_dir="$(detect_install_dir)"
    install_binary "${version}" "${platform}"

    # Verify installation
    verify_installation "${install_dir}"

    # Show usage information
    log_success "Get started with:"
    log_success "    ${BINARY_NAME} --help"
    log_success "    ${BINARY_NAME} version"
    log_success "Happy coding with Tiger CLI! 🐅"
}

# Run main function
main "$@"
