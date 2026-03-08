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
LATEST_TAG=""
CURRENT_VERSION=""

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

fetch_latest_version() {
    api_url="https://api.github.com/repos/CarlosRGL/sitesync/releases/latest"
    LATEST_TAG=$(curl -fsSL "$api_url" 2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
}

detect_current_version() {
    current_bin=""
    if [ -x "${INSTALL_DIR}/${BINARY}" ]; then
        current_bin="${INSTALL_DIR}/${BINARY}"
    else
        current_bin=$(command -v "$BINARY" 2>/dev/null || true)
    fi

    if [ -n "$current_bin" ] && [ -x "$current_bin" ]; then
        CURRENT_VERSION=$($current_bin version 2>/dev/null | awk 'NR==1 {print $2}')
    fi
}

show_version_plan() {
    target="${LATEST_TAG:-unknown}"
    if [ -n "$CURRENT_VERSION" ]; then
        info "Current version: ${CURRENT_VERSION}"
        info "Target version: ${target}"
        if [ "$CURRENT_VERSION" = "${LATEST_TAG#v}" ] || [ "$CURRENT_VERSION" = "$LATEST_TAG" ]; then
            info "Reinstalling ${CURRENT_VERSION}"
        else
            info "Upgrading ${CURRENT_VERSION} -> ${target}"
        fi
    else
        info "Current version: not installed"
        info "Target version: ${target}"
    fi
}

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

# ── PATH patch ───────────────────────────────────────────────────────────────
detect_shell_profile() {
    if [ -n "$SHELL" ]; then
        case "$SHELL" in
            */zsh)
                for f in "$HOME/.zshrc" "$HOME/.zprofile"; do
                    [ -f "$f" ] && printf "%s" "$f" && return
                done ;;
            */bash)
                for f in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
                    [ -f "$f" ] && printf "%s" "$f" && return
                done ;;
            */fish)
                f="$HOME/.config/fish/config.fish"
                [ -f "$f" ] && printf "%s" "$f" && return ;;
        esac
    fi
    # Fallback
    for f in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
        [ -f "$f" ] && printf "%s" "$f" && return
    done
}

patch_path() {
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*)
            return 0 ;;
    esac

    SHELL_PROFILE=$(detect_shell_profile)

    if [ -z "$SHELL_PROFILE" ]; then
        warn "${INSTALL_DIR} is not in your PATH"
        printf "  Add this to your shell profile:\n"
        printf "  ${CYAN}export PATH=\"%s:\$PATH\"${RESET}\n" "$INSTALL_DIR"
        return
    fi

    # Already patched in file?
    if grep -qF "$INSTALL_DIR" "$SHELL_PROFILE" 2>/dev/null; then
        ok "${INSTALL_DIR} already referenced in $(basename "$SHELL_PROFILE")"
        return
    fi

    printf "\n# Added by sitesync installer\nexport PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR" >> "$SHELL_PROFILE"
    ok "Added ${INSTALL_DIR} to PATH in $(basename "$SHELL_PROFILE")"
    printf "  ${YELLOW}→ Reload with: source %s${RESET}\n" "$SHELL_PROFILE"
}

default_config_dir() {
    if [ -n "${SITESYNC_ETC:-}" ]; then
        printf "%s" "$SITESYNC_ETC"
    else
        printf "%s" "$HOME/.config/sitesync"
    fi
}

valid_config_name() {
    case "$1" in
        ""|"."|".."|*/*|*\\*)
            return 1 ;;
        *)
            return 0 ;;
    esac
}

patch_sitesync_etc() {
    CONFIG_DIR=$(default_config_dir)
    export SITESYNC_ETC="$CONFIG_DIR"
    mkdir -p "$CONFIG_DIR"

    SHELL_PROFILE=$(detect_shell_profile)
    EXPORT_LINE="export SITESYNC_ETC=\"${CONFIG_DIR}\""

    if [ -z "$SHELL_PROFILE" ]; then
        warn "Could not detect a shell profile for SITESYNC_ETC"
        printf "  Add this to your shell profile:\n"
        printf "  ${CYAN}%s${RESET}\n" "$EXPORT_LINE"
        return
    fi

    if grep -q "SITESYNC_ETC" "$SHELL_PROFILE" 2>/dev/null; then
        ok "SITESYNC_ETC already referenced in $(basename "$SHELL_PROFILE")"
        return
    fi

    printf "\n# Added by sitesync installer\n%s\n" "$EXPORT_LINE" >> "$SHELL_PROFILE"
    ok "Added SITESYNC_ETC to $(basename "$SHELL_PROFILE")"
    printf "  ${YELLOW}→ Reload with: source %s${RESET}\n" "$SHELL_PROFILE"
}

write_starter_config() {
    site_name="$1"
    config_dir=$(default_config_dir)
    target_dir="${config_dir}/${site_name}"
    target_file="${target_dir}/config.toml"
    remote_root="/var/www/${site_name}"
    local_root="${HOME}/Sites/${site_name}"

    if [ -f "$target_file" ]; then
        warn "Starter config already exists at ${target_file}"
        return
    fi

    mkdir -p "$target_dir"
    cat > "$target_file" <<EOF
[site]
name = "${site_name}"
description = "Starter config generated by the sitesync installer"

[source]
server = "www.example.com"
user = "deploy"
port = 22
type = "remote_base"
file = ""
compress = true
db_hostname = "localhost"
db_port = ""
db_name = "${site_name}_prod"
db_user = "db_user"
db_password = "db_password"
site_protocol = "https://"
site_host = "www.example.com"
site_slug = ""
files_root = "${remote_root}"
path_to_mysqldump = "mysqldump"
remote_nice = ""

[destination]
site_protocol = "http://"
site_host = "${site_name}.local"
site_slug = ""
files_root = "${local_root}"
db_hostname = "localhost"
db_port = ""
db_name = "${site_name}_local"
db_user = "root"
db_password = ""
path_to_mysql = "mysql"
path_to_mysqldump = "mysqldump"
path_to_rsync = "rsync"
path_to_lftp = "lftp"
local_nice = ""

[database]
sql_options_structure = "--default-character-set=utf8"
sql_options_extra = ""
ignore_tables = []

[[replace]]
search = "https://www.example.com"
replace = "http://${site_name}.local"

[[replace]]
search = "${remote_root}"
replace = "${local_root}"

[[sync]]
src = "${remote_root}"
dst = "${local_root}"

[transport]
type = "rsync"
rsync_options = "-uvrpztl"
exclude = ["/sitesync/", ".git/", ".svn/", ".DS_Store"]

[transport.lftp]
password = ""
port = 21
connect_options = ""
mirror_options = "--parallel=16 --verbose --only-newer"

[hooks]
path = "hook"

[logging]
file = "log/sitesync.log"
EOF

    ok "Created starter config at ${target_file}"
}

bootstrap_site_config() {
    if [ ! -t 0 ] || [ ! -t 1 ]; then
        info "Skipping single-site bootstrap prompts (non-interactive install)"
        return
    fi

    printf "\n"
    info "Config bootstrap"
    printf "  Bootstrap a starter config for one site now? [y/N]: "
    read -r answer || return
    answer=$(printf "%s" "$answer" | tr '[:upper:]' '[:lower:]')
    case "$answer" in
        y|yes) ;;
        *)
            info "Skipping starter config bootstrap"
            return ;;
    esac

    while :; do
        printf "  Site name: "
        read -r site_name || return
        if valid_config_name "$site_name"; then
            break
        fi
        warn "Invalid site name. Use a non-empty name without path separators."
    done

    patch_sitesync_etc
    write_starter_config "$site_name"
}

# ── Main ─────────────────────────────────────────────────────────────────────
main() {
    printf "\n${BOLD}${CYAN}⚡ sitesync installer${RESET}\n\n"

    detect_platform
    info "Platform: ${PLATFORM}"
    fetch_latest_version
    detect_current_version
    show_version_plan
    printf "\n"

    info "Checking dependencies..."
    check_deps
    printf "\n"

    get_binary
    install_binary
    printf "\n"

    patch_path
    printf "\n"
    bootstrap_site_config

    printf "\n${GREEN}${BOLD}── Installed! ──${RESET}\n"
    printf "  Run ${CYAN}${BINARY} setup${RESET} to configure your environment or add more sites.\n"
    printf "  Run ${CYAN}${BINARY} --help${RESET} for usage.\n\n"
}

main
