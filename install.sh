#!/bin/sh
#
# sitesync installer
#
# Usage:
#   curl -fsSL https://github.com/CarlosRGL/sitesync/releases/latest/download/install.sh | sh
#
# Or download manually and run:
#   sh install.sh
#
set -e

# ── Config ───────────────────────────────────────────────────────────────────
GITHUB_REPO="https://github.com/CarlosRGL/sitesync"
GITLAB_REPO="https://gitlab.quai13.net/teamtreize/sitesync"
BINARY="sitesync"
INSTALL_DIR="${INSTALL_DIR:-$HOME/bin}"

# ── Colors ───────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
    BOLD='\033[1m'
    CYAN='\033[36m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    RED='\033[31m'
    DIM='\033[2m'
    RESET='\033[0m'
else
    BOLD='' CYAN='' GREEN='' YELLOW='' RED='' DIM='' RESET=''
fi

info()  { printf "${CYAN}▸${RESET} %s\n" "$1"; }
ok()    { printf "${GREEN}✔${RESET} %s\n" "$1"; }
warn()  { printf "${YELLOW}⚠${RESET} %s\n" "$1"; }
fail()  { printf "${RED}✘${RESET} %s\n" "$1"; exit 1; }

# ── Detect OS / arch ────────────────────────────────────────────────────────
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin) OS="darwin" ;;
        linux)  OS="linux" ;;
        *)      fail "Unsupported OS: $OS (only macOS and Linux are supported)" ;;
    esac

    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        arm64|aarch64)  ARCH="arm64" ;;
        *)              fail "Unsupported architecture: $ARCH" ;;
    esac

    PLATFORM="${OS}-${ARCH}"
}

# ── Check dependencies ──────────────────────────────────────────────────────
check_deps() {
    for cmd in rsync mysql mysqldump ssh; do
        if command -v "$cmd" >/dev/null 2>&1; then
            ok "$cmd found"
        else
            warn "$cmd not found — needed for sync operations"
        fi
    done
}

# ── Download or build ───────────────────────────────────────────────────────
get_binary() {
    DOWNLOAD_URL="${GITHUB_REPO}/releases/latest/download/${BINARY}-${PLATFORM}"

    info "Downloading ${BINARY} for ${PLATFORM}..."

    # Try downloading a pre-built binary first
    TMPFILE=$(mktemp)
    if curl -fsSL -o "$TMPFILE" "$DOWNLOAD_URL" 2>/dev/null; then
        chmod +x "$TMPFILE"
        ok "Downloaded pre-built binary"
        return 0
    fi

    rm -f "$TMPFILE"

    # No pre-built binary available — try building from source
    if command -v go >/dev/null 2>&1; then
        info "No pre-built binary found. Building from source..."
        build_from_source
        return 0
    fi

    # Neither works
    printf "\n"
    fail "No pre-built binary available and Go is not installed.

  To install Go: https://go.dev/dl/

  Or ask your team to run 'make release publish' to upload binaries
  to a GitHub release at:
    ${GITHUB_REPO}/releases

  Then re-run this installer."
}

build_from_source() {
    TMPDIR_SRC=$(mktemp -d)
    info "Cloning repository..."
    git clone --depth 1 "${GITLAB_REPO}.git" "$TMPDIR_SRC" 2>/dev/null || \
        git clone --depth 1 "ssh://git@gitlab.quai13.net:2221/teamtreize/sitesync.git" "$TMPDIR_SRC"

    cd "$TMPDIR_SRC"
    TMPFILE=$(mktemp)
    info "Building..."
    go build -ldflags "-s -w" -o "$TMPFILE" ./cmd/sitesync
    chmod +x "$TMPFILE"
    cd - >/dev/null
    rm -rf "$TMPDIR_SRC"
    ok "Built from source"
}

# ── Install ──────────────────────────────────────────────────────────────────
install_binary() {
    mkdir -p "$INSTALL_DIR"
    mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
    ok "Installed to ${INSTALL_DIR}/${BINARY}"
}

# ── PATH check ───────────────────────────────────────────────────────────────
check_path() {
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            warn "${INSTALL_DIR} is not in your PATH"
            printf "  Add this to your shell profile:\n"
            printf "  ${CYAN}export PATH=\"%s:\$PATH\"${RESET}\n" "$INSTALL_DIR"
            ;;
    esac
}

# ── Main ─────────────────────────────────────────────────────────────────────
main() {
    printf "\n${BOLD}${CYAN}⚡ sitesync installer${RESET}\n\n"

    detect_platform
    info "Platform: ${PLATFORM}"
    printf "\n"

    info "Checking dependencies..."
    check_deps
    printf "\n"

    get_binary
    install_binary
    printf "\n"

    check_path

    printf "\n${GREEN}${BOLD}── Installed! ──${RESET}\n"
    printf "  Run ${CYAN}${BINARY} setup${RESET} to configure your environment.\n"
    printf "  Run ${CYAN}${BINARY} --help${RESET} for usage.\n\n"
}

main
