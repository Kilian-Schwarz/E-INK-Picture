#!/bin/bash
# E-Ink Picture — one-command bootstrap installer.
#
#   curl -fsSL https://raw.githubusercontent.com/Kilian-Schwarz/E-INK-Picture/main/install.sh | bash
#
# Thin bootstrap only: gates OS/arch, ensures git, clones (or refreshes) the
# repo into ${EINK_INSTALL_DIR:-$HOME/E-INK-Picture} and delegates everything
# else to setup.sh --headless. No setup logic is duplicated here.
#
# Flags (forwarded to setup.sh):
#   --update              force update mode (auto-detected for existing checkouts)
#   --allow-preview-only  tolerate display driver failures on Pi hardware
#   --dry-run             print every mutating action instead of executing it
#
# Environment:
#   EINK_INSTALL_DIR   target directory (default: $HOME/E-INK-Picture)
#   EINK_REPO_URL      repo to clone (default: GitHub upstream; file:// works for tests)
#   EINK_REPO_BRANCH   branch to clone (default: main)
#   EINK_TEST_OS/EINK_TEST_ARCH  platform overrides, honored ONLY with --dry-run
set -euo pipefail

REPO_URL="${EINK_REPO_URL:-https://github.com/Kilian-Schwarz/E-INK-Picture.git}"
REPO_BRANCH="${EINK_REPO_BRANCH:-main}"
INSTALL_DIR="${EINK_INSTALL_DIR:-$HOME/E-INK-Picture}"

DRY_RUN=false
FORWARD_UPDATE=false
FORWARD_ALLOW_PREVIEW_ONLY=false

info()  { echo "[INFO] ${1:-}"; }
error() { echo "[ERROR] ${1:-}" >&2; }

# Every mutating command goes through run(): with --dry-run it is printed
# as "[DRY-RUN] <cmd>" instead of being executed.
run() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] $*"
        return 0
    fi
    "$@"
}

parse_flags() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --dry-run)            DRY_RUN=true ;;
            --update)             FORWARD_UPDATE=true ;;
            --allow-preview-only) FORWARD_ALLOW_PREVIEW_ONLY=true ;;
            --headless|--yes)     : ;; # this bootstrap is always headless
            *)
                error "Unknown option: $1"
                error "Usage: install.sh [--update] [--allow-preview-only] [--dry-run]"
                exit 2
                ;;
        esac
        shift
    done
}

# EINK_TEST_* overrides are honored ONLY with --dry-run (Mac testability);
# real installations always use uname.
detect_os() {
    if [ "$DRY_RUN" = true ] && [ -n "${EINK_TEST_OS:-}" ]; then
        echo "$EINK_TEST_OS"
    else
        uname -s
    fi
}

detect_arch() {
    if [ "$DRY_RUN" = true ] && [ -n "${EINK_TEST_ARCH:-}" ]; then
        echo "$EINK_TEST_ARCH"
    else
        uname -m
    fi
}

check_platform() {
    local os arch
    os="$(detect_os)"
    arch="$(detect_arch)"
    if [ "$os" != "Linux" ]; then
        error "This installer only supports Linux (got: $os). See INSTALL.md for the manual route."
        exit 1
    fi
    case "$arch" in
        aarch64|armv7l|armv6l|x86_64) : ;;
        *)
            error "Unsupported architecture: $arch (supported: aarch64, armv7l, armv6l, x86_64)"
            exit 1
            ;;
    esac
    info "Platform: $os $arch"
}

# Fail early and loudly if sudo would prompt for a password — a pipe from
# curl has no TTY to answer it on.
check_sudo() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] sudo -n true (verify passwordless sudo)"
        return 0
    fi
    if ! sudo -n true 2>/dev/null; then
        error "sudo needs a password, but this installer runs non-interactively."
        error "Either run 'sudo -v' first and retry the one-liner,"
        error "or download install.sh and run it directly in a terminal."
        exit 1
    fi
}

ensure_git() {
    if command -v git >/dev/null 2>&1; then
        return 0
    fi
    info "git not found — installing via apt..."
    run sudo apt-get update -qq
    run sudo apt-get install -y -qq git
}

clone_or_update() {
    if [ -e "$INSTALL_DIR/.git" ]; then
        info "Existing checkout at $INSTALL_DIR — switching to update mode"
        FORWARD_UPDATE=true
        return 0
    fi
    info "Cloning $REPO_URL (branch $REPO_BRANCH) into $INSTALL_DIR"
    run git clone --depth 1 --branch "$REPO_BRANCH" "$REPO_URL" "$INSTALL_DIR"
}

run_setup() {
    local -a setup_args
    setup_args=(--headless)
    if [ "$FORWARD_UPDATE" = true ]; then
        setup_args+=(--update)
    fi
    if [ "$FORWARD_ALLOW_PREVIEW_ONLY" = true ]; then
        setup_args+=(--allow-preview-only)
    fi
    if [ "$DRY_RUN" = true ]; then
        setup_args+=(--dry-run)
    fi
    if [ -f "$INSTALL_DIR/setup.sh" ]; then
        info "Delegating to setup.sh ${setup_args[*]}"
        bash "$INSTALL_DIR/setup.sh" "${setup_args[@]}"
    elif [ "$DRY_RUN" = true ]; then
        # Repo not cloned in dry-run mode: print the planned delegation.
        echo "[DRY-RUN] bash $INSTALL_DIR/setup.sh ${setup_args[*]}"
    else
        error "setup.sh not found in $INSTALL_DIR — clone incomplete?"
        exit 1
    fi
}

main() {
    parse_flags "$@"
    info "E-Ink Picture — one-command installer"
    check_platform
    check_sudo
    ensure_git
    clone_or_update
    run_setup
}

main "$@"
