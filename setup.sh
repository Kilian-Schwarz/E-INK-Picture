#!/bin/bash
# E-Ink Picture — native Raspberry Pi setup (server build + client venv + systemd).
#
# Usage: ./setup.sh [--headless|--yes] [--update] [--allow-preview-only] [--dry-run]
#
#   --headless, --yes     no prompts; documented defaults: enable SPI = yes,
#                         install systemd services = yes. Auto-enabled when
#                         stdin is not a TTY (curl | bash).
#   --update              update an existing install: git pull --ff-only,
#                         rebuild server, sync pinned deps, restart services.
#   --allow-preview-only  on Pi hardware, continue despite display driver
#                         failures (client then runs in preview-only mode).
#   --dry-run             print every mutating action instead of executing it.
#
# Test overrides (honored ONLY with --dry-run or when sourced by tests):
#   EINK_TEST_OS, EINK_TEST_ARCH, EINK_TEST_PI_MODEL, EINK_TEST_DOCKER_PS
set -euo pipefail

# true when sourced (e.g. by scripts/test-setup.sh): main() will not run then.
if [ "${BASH_SOURCE[0]:-$0}" != "$0" ]; then
    EINK_SOURCED=true
else
    EINK_SOURCED=false
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

# ----- Constants -----
GO_VERSION="1.24.1"
REQUIRED_GO_MAJOR=1
REQUIRED_GO_MINOR=24

WAVESHARE_EPD_REPO="https://github.com/waveshareteam/e-Paper.git"
WAVESHARE_EPD_PIN="500fa7c6f57b786102cccb866682f8cc43e08996"
WAVESHARE_EPD_SUBDIR="RaspberryPi_JetsonNano/python"

# Must match client/requirements.txt exactly (lower bounds shared with CI).
REQUESTS_CONSTRAINT="requests>=2.31.0,<3"
PILLOW_CONSTRAINT="Pillow>=10.0.0,<13"

# lgpio is handled separately (see install_lgpio), NOT pip-built here: on the
# target Python (3.13) its sdist needs the swig toolchain and the pip build
# fails (HIL-3). The distro package python3-lgpio matches the system Python and
# is exposed to the venv via --system-site-packages.
GPIO_PACKAGES=("RPi.GPIO>=0.7.1" "spidev>=3.6" "gpiod>=2.0.2" "gpiozero>=2.0")
RPI_GPIO_CONSTRAINT="RPi.GPIO>=0.7.1"

# Pin factory (gpiozero): on Raspberry Pi OS with kernel >= 6.6 (Bookworm/Trixie)
# lgpio is the ONLY working factory. The legacy 'native' sysfs factory is gone
# (OSError [Errno 22]), RPi.GPIO fails edge detection, pigpio needs a daemon —
# HIL-3 settled this empirically. rpigpio stays only as a documented pre-6.6
# fallback. See specs/E2.5a-native-gpio-fix.md.
LGPIO_APT_PACKAGE="python3-lgpio"
PIN_FACTORY_DEFAULT="lgpio"
PIN_FACTORY_FALLBACK="rpigpio"

# The pinned waveshareteam/e-Paper package drags in Jetson.GPIO, whose RPi/GPIO
# shim shadows the real RPi.GPIO (HIL-3). It is removed after the driver install.
JETSON_GPIO_PACKAGE="Jetson.GPIO"

ESSENTIAL_APT_PACKAGES=(python3 python3-pip python3-venv python3-dev git curl wget
    libopenjp2-7-dev libjpeg-dev zlib1g-dev)
OPTIONAL_APT_PACKAGES=(libtiff-dev libfreetype6-dev libgpiod2 libgpiod-dev
    fonts-noto-core fonts-dejavu-core fontconfig logrotate)

SYSTEMD_UNITS=("eink-server.service" "eink-client.service")

VENV_DIR="$SCRIPT_DIR/venv"

# ----- Mutable state (set by parse_flags / the pipeline) -----
HEADLESS=false
UPDATE_MODE=false
ALLOW_PREVIEW_ONLY=false
DRY_RUN=false
PREVIEW_ONLY=false
REBOOT_REQUIRED=false
FAILED_GPIO_PACKAGES=""
PIN_FACTORY=""
OS=""
ARCH=""
GO_ARCH=""
GO_DL_ARCH=""
GO_ARM=""

# ----- Output helpers -----
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { printf '%b[INFO]%b %s\n' "$GREEN" "$NC" "${1:-}"; }
warn()  { printf '%b[WARN]%b %s\n' "$YELLOW" "$NC" "${1:-}"; }
error() { printf '%b[ERROR]%b %s\n' "$RED" "$NC" "${1:-}" >&2; }

# Every mutating command goes through run(): with --dry-run it is printed
# as "[DRY-RUN] <cmd>" instead of being executed.
run() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] $*"
        return 0
    fi
    "$@"
}

# EINK_TEST_* detection overrides are honored ONLY in dry-run or source mode,
# so real installations can never be steered by stray environment variables.
test_overrides_active() {
    [ "$EINK_SOURCED" = true ] || [ "$DRY_RUN" = true ]
}

# ----- Flag parsing -----
parse_flags() {
    HEADLESS=false
    UPDATE_MODE=false
    ALLOW_PREVIEW_ONLY=false
    DRY_RUN=false
    while [ $# -gt 0 ]; do
        case "$1" in
            --headless|--yes)     HEADLESS=true ;;
            --update)             UPDATE_MODE=true ;;
            --allow-preview-only) ALLOW_PREVIEW_ONLY=true ;;
            --dry-run)            DRY_RUN=true ;;
            *)
                error "Unknown option: $1"
                error "Usage: ./setup.sh [--headless|--yes] [--update] [--allow-preview-only] [--dry-run]"
                return 2
                ;;
        esac
        shift
    done
    return 0
}

# curl | bash makes stdin a pipe — read would consume script text (or fail),
# so a non-TTY stdin always forces headless mode.
auto_headless() {
    if [ "$HEADLESS" != true ] && [ ! -t 0 ]; then
        HEADLESS=true
        info "stdin is not a TTY — switching to headless mode"
    fi
}

# Interactive yes/no prompt. In headless mode the documented default is
# returned WITHOUT touching stdin (this is the interactive guard for read).
confirm() {
    local prompt="$1" default="$2" reply
    if [ "$HEADLESS" = true ]; then
        if [ "$default" = "y" ]; then
            return 0
        fi
        return 1
    fi
    read -r -p "$prompt " reply
    if [ -z "$reply" ]; then
        reply="$default"
    fi
    case "$reply" in
        [Yy]*) return 0 ;;
        *)     return 1 ;;
    esac
}

# ----- Platform detection -----
detect_os() {
    if test_overrides_active && [ -n "${EINK_TEST_OS:-}" ]; then
        echo "$EINK_TEST_OS"
    else
        uname -s
    fi
}

detect_arch() {
    if test_overrides_active && [ -n "${EINK_TEST_ARCH:-}" ]; then
        echo "$EINK_TEST_ARCH"
    else
        uname -m
    fi
}

pi_model() {
    if test_overrides_active && [ -n "${EINK_TEST_PI_MODEL:-}" ]; then
        echo "$EINK_TEST_PI_MODEL"
    elif [ -r /proc/device-tree/model ]; then
        tr -d '\0' < /proc/device-tree/model
    else
        echo ""
    fi
}

is_raspberry_pi() {
    case "$(pi_model)" in
        *"Raspberry Pi"*) return 0 ;;
    esac
    return 1
}

kernel_release() {
    if test_overrides_active && [ -n "${EINK_TEST_KERNEL:-}" ]; then
        echo "$EINK_TEST_KERNEL"
    else
        uname -r
    fi
}

# 0 = kernel ($1, e.g. "6.12.47" or "6.12.47-rpt-rpi") is >= 6.6 — the target
# where lgpio is mandatory (sysfs/native gone). An unparseable version is
# treated as modern (>= 6.6) so a missing lgpio fails loudly instead of
# silently selecting a broken factory.
kernel_ge_66() {
    local rel="${1:-}" major minor
    rel="${rel%%-*}"          # strip -rpt-rpi / -v8 suffixes
    major="${rel%%.*}"
    minor="${rel#*.}"
    minor="${minor%%.*}"
    case "$major" in ''|*[!0-9]*) return 0 ;; esac
    case "$minor" in ''|*[!0-9]*) minor=0 ;; esac
    if [ "$major" -gt 6 ]; then return 0; fi
    if [ "$major" -eq 6 ] && [ "$minor" -ge 6 ]; then return 0; fi
    return 1
}

# Maps `uname -m` to "<GOARCH> <GO_DL_ARCH> <GOARM>" (GOARM "-" = unset).
# armv7l deliberately uses the armv6l tarball: go.dev ships no armv7 build.
map_architecture() {
    case "$1" in
        aarch64) echo "arm64 arm64 -" ;;
        armv7l)  echo "arm armv6l 7" ;;
        armv6l)  echo "arm armv6l 6" ;;
        x86_64)  echo "amd64 amd64 -" ;;
        *)       return 1 ;;
    esac
}

go_tarball_name() {
    echo "go${GO_VERSION}.linux-${1}.tar.gz"
}

setup_platform() {
    OS="$(detect_os)"
    ARCH="$(detect_arch)"
    if [ "$OS" != "Linux" ]; then
        error "This script only supports Linux (got: $OS)"
        exit 1
    fi
    local mapping rest
    if ! mapping="$(map_architecture "$ARCH")"; then
        error "Unsupported architecture: $ARCH (supported: aarch64, armv7l, armv6l, x86_64)"
        exit 1
    fi
    GO_ARCH="${mapping%% *}"
    rest="${mapping#* }"
    GO_DL_ARCH="${rest%% *}"
    GO_ARM="${rest##* }"
    if [ "$GO_ARM" = "-" ]; then
        GO_ARM=""
    fi
    info "Platform: $OS $ARCH (Go: $GO_ARCH, tarball: $(go_tarball_name "$GO_DL_ARCH"))"
    if is_raspberry_pi; then
        info "Raspberry Pi detected: $(pi_model)"
    fi
}

# ----- Preflight (before ANY mutation) -----
docker_ps_output() {
    if test_overrides_active && [ -n "${EINK_TEST_DOCKER_PS+x}" ]; then
        echo "${EINK_TEST_DOCKER_PS}"
        return 0
    fi
    if ! command -v docker >/dev/null 2>&1; then
        return 0
    fi
    docker ps --format '{{.Names}} {{.Image}}' 2>/dev/null || true
}

# Pure decision function ($1 = docker ps output): 0 = conflict found.
docker_conflict() {
    printf '%s\n' "${1:-}" | grep -Eiq 'eink|e-ink-picture'
}

port_in_use() {
    if ! command -v ss >/dev/null 2>&1; then
        return 1
    fi
    ss -Hltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${1}\$"
}

preflight() {
    local ps_out
    ps_out="$(docker_ps_output)"
    if docker_conflict "$ps_out"; then
        error "A Docker container of this project appears to be running:"
        printf '%s\n' "$ps_out" | grep -Ei 'eink|e-ink-picture' >&2 || true
        error "Native and Docker mode conflict (port ${PORT:-5000} and SPI/GPIO devices)."
        error "Stop it first: run 'docker compose down' in the old checkout and back up / migrate its data/ directory."
        exit 1
    fi
    # In update mode the port is expected to be held by our own service,
    # which is restarted at the end of the run.
    if [ "$UPDATE_MODE" != true ] && port_in_use "${PORT:-5000}"; then
        error "Port ${PORT:-5000} is already in use by another process."
        error "Stop it (or set PORT in .env) and re-run. If this is the old Docker install: 'docker compose down'."
        exit 1
    fi
    info "Preflight OK (no Docker conflict, port ${PORT:-5000} available)"
}

# ----- Update mode -----
update_checkout() {
    if [ "$UPDATE_MODE" != true ]; then
        return 0
    fi
    info "Update mode: pulling latest changes..."
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] git -C $SCRIPT_DIR pull --ff-only"
        return 0
    fi
    if ! git -C "$SCRIPT_DIR" pull --ff-only; then
        error "git pull --ff-only failed — local changes or diverged history."
        error "Resolve manually (git status / git stash), then re-run. No 'reset --hard' is ever performed."
        exit 1
    fi
}

# ----- System packages -----
install_system_packages() {
    info "Installing system packages..."
    run sudo apt-get update -qq
    if ! run sudo apt-get install -y -qq "${ESSENTIAL_APT_PACKAGES[@]}"; then
        error "Failed to install essential system packages: ${ESSENTIAL_APT_PACKAGES[*]}"
        error "Fix apt (network? sources?) and re-run — continuing would only fail later."
        exit 1
    fi
    if ! run sudo apt-get install -y -qq "${OPTIONAL_APT_PACKAGES[@]}"; then
        warn "Some optional packages failed as a batch — retrying individually..."
        local pkg
        for pkg in "${OPTIONAL_APT_PACKAGES[@]}"; do
            if ! run sudo apt-get install -y -qq "$pkg"; then
                warn "Optional package not installed: $pkg"
            fi
        done
    fi
}

# ----- SPI -----
spi_present() {
    ls /dev/spidev* >/dev/null 2>&1
}

# Picks the boot config file ($1 = optional root prefix, used by tests).
spi_config_path() {
    local root="${1:-}"
    if [ -f "$root/boot/firmware/config.txt" ]; then
        echo "$root/boot/firmware/config.txt"
    elif [ -f "$root/boot/config.txt" ]; then
        echo "$root/boot/config.txt"
    else
        return 1
    fi
}

spi_line_active() {
    grep -Eq '^[[:space:]]*dtparam=spi=on' "$1"
}

# Idempotent: appends dtparam=spi=on exactly once (commented lines don't count).
ensure_spi_config_line() {
    local cfg="$1"
    if spi_line_active "$cfg"; then
        return 0
    fi
    if [ -w "$cfg" ]; then
        echo "dtparam=spi=on" >> "$cfg"
    else
        echo "dtparam=spi=on" | sudo tee -a "$cfg" >/dev/null
    fi
}

enable_spi() {
    if spi_present; then
        info "SPI already enabled"
        return 0
    fi
    if ! confirm "Enable SPI now (required for the display)? [Y/n]" y; then
        warn "SPI left disabled — the display client cannot drive the panel until SPI is enabled"
        return 0
    fi
    if command -v raspi-config >/dev/null 2>&1; then
        run sudo raspi-config nonint do_spi 0
        REBOOT_REQUIRED=true
        info "SPI enabled via raspi-config (active after reboot)"
        return 0
    fi
    local cfg
    if cfg="$(spi_config_path)"; then
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] ensure 'dtparam=spi=on' in $cfg"
        else
            ensure_spi_config_line "$cfg"
        fi
        REBOOT_REQUIRED=true
        info "SPI enabled via $cfg (active after reboot)"
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] enable SPI (sudo raspi-config nonint do_spi 0, or dtparam=spi=on in /boot/firmware/config.txt or /boot/config.txt)"
        REBOOT_REQUIRED=true
        return 0
    fi
    warn "Could not enable SPI automatically (no raspi-config, no boot config.txt) — enable it manually"
}

# ----- Go toolchain -----
go_needs_install() {
    if test_overrides_active && [ -n "${EINK_TEST_ARCH:-}" ]; then
        # Test override active: the host Go is not the target machine's Go,
        # so always plan the install (keeps dry-run plans deterministic).
        return 0
    fi
    if ! command -v go >/dev/null 2>&1; then
        return 0
    fi
    local ver major minor
    ver="$(go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')"
    major="${ver%%.*}"
    minor="${ver##*.}"
    if [ "$major" -gt "$REQUIRED_GO_MAJOR" ]; then
        info "Go $ver already installed (>= $REQUIRED_GO_MAJOR.$REQUIRED_GO_MINOR)"
        return 1
    fi
    if [ "$major" -eq "$REQUIRED_GO_MAJOR" ] && [ "$minor" -ge "$REQUIRED_GO_MINOR" ]; then
        info "Go $ver already installed (>= $REQUIRED_GO_MAJOR.$REQUIRED_GO_MINOR)"
        return 1
    fi
    warn "Go $ver too old (need >= $REQUIRED_GO_MAJOR.$REQUIRED_GO_MINOR)"
    return 0
}

install_go() {
    local tarball url
    tarball="$(go_tarball_name "$GO_DL_ARCH")"
    url="https://go.dev/dl/${tarball}"
    info "Installing Go ${GO_VERSION} from ${url}"
    if ! run wget -q -O "/tmp/${tarball}" "$url"; then
        error "Go download failed: $url"
        error "Check your internet connection, or install Go >= ${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR} manually."
        exit 1
    fi
    run sudo rm -rf /usr/local/go
    run sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    run rm -f "/tmp/${tarball}"
    if ! grep -q '/usr/local/go/bin' "$HOME/.profile" 2>/dev/null; then
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] append 'export PATH=/usr/local/go/bin:\$PATH' to ~/.profile"
        else
            # shellcheck disable=SC2016  # $PATH must stay literal: expanded by the login shell, not here
            echo 'export PATH="/usr/local/go/bin:$PATH"' >> "$HOME/.profile"
            info "Added Go to PATH in ~/.profile"
        fi
    fi
}

ensure_go() {
    if go_needs_install; then
        install_go
    fi
}

build_server() {
    info "Building Go server (this may take several minutes on a Pi Zero)..."
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] (cd server && CGO_ENABLED=0 GOOS=linux GOARCH=$GO_ARCH${GO_ARM:+ GOARM=$GO_ARM} go mod download && go build -ldflags='-s -w' -o eink-server .)"
        return 0
    fi
    (
        cd "$SCRIPT_DIR/server"
        export CGO_ENABLED=0 GOOS=linux GOARCH="$GO_ARCH"
        if [ -n "$GO_ARM" ]; then
            export GOARM="$GO_ARM"
        fi
        go mod download
        go build -ldflags="-s -w" -o "$SCRIPT_DIR/server/eink-server" .
    )
    info "Server binary built: server/eink-server ($(du -h "$SCRIPT_DIR/server/eink-server" | cut -f1))"
}

# ----- Python venv + core packages -----
# 0 = pyvenv.cfg ($1) enables system site-packages (needed so the apt-installed
# python3-lgpio in the system dist-packages is importable inside the venv).
venv_has_system_site() {
    grep -Eiq '^[[:space:]]*include-system-site-packages[[:space:]]*=[[:space:]]*true' "$1" 2>/dev/null
}

setup_venv() {
    if [ -d "$VENV_DIR" ]; then
        if [ "$DRY_RUN" = true ] || venv_has_system_site "$VENV_DIR/pyvenv.cfg"; then
            info "Virtual environment already exists (reused)"
            return 0
        fi
        # An old venv without --system-site-packages cannot see the system
        # python3-lgpio, so the lgpio pin factory would be unavailable.
        # Recreate it once so the E2.5a fix applies on update.
        warn "Recreating venv with --system-site-packages (required for the system lgpio pin factory)"
        run rm -rf "$VENV_DIR"
    fi
    run python3 -m venv --system-site-packages "$VENV_DIR"
    run "$VENV_DIR/bin/pip" install --no-cache-dir --upgrade pip setuptools wheel
}

constraints_signature() {
    printf '%s|%s' "$REQUESTS_CONSTRAINT" "$PILLOW_CONSTRAINT"
}

# Idempotency guard ($1 = marker file): 0 = pip install needed.
core_python_needed() {
    if [ ! -f "$1" ]; then
        return 0
    fi
    [ "$(cat "$1" 2>/dev/null)" != "$(constraints_signature)" ]
}

install_core_python() {
    local marker="$VENV_DIR/.constraints"
    if [ "$DRY_RUN" != true ] && ! core_python_needed "$marker"; then
        info "Core Python packages up to date (skipped)"
        return 0
    fi
    info "Installing core Python packages..."
    if ! run "$VENV_DIR/bin/pip" install --no-cache-dir "$REQUESTS_CONSTRAINT" "$PILLOW_CONSTRAINT"; then
        error "Failed to install core Python packages ($REQUESTS_CONSTRAINT, $PILLOW_CONSTRAINT)"
        exit 1
    fi
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] write constraints marker $marker"
    else
        constraints_signature > "$marker"
    fi
}

# ----- Display driver stack (Pi only) -----
# Policy ($1 = is_pi, $2 = allow_preview_only): echoes "fatal" or "preview-only".
decide_driver_failure() {
    if [ "$1" = "true" ] && [ "$2" != "true" ]; then
        echo "fatal"
    else
        echo "preview-only"
    fi
}

apply_driver_policy() {
    local step="$1" is_pi=false decision
    if is_raspberry_pi; then
        is_pi=true
    fi
    decision="$(decide_driver_failure "$is_pi" "$ALLOW_PREVIEW_ONLY")"
    if [ "$decision" = "fatal" ]; then
        error "Display driver setup failed: $step"
        error "Refusing to finish silently in preview-only mode on Raspberry Pi hardware."
        error "Fix the error above, or re-run with --allow-preview-only to continue without the display."
        exit 1
    fi
    PREVIEW_ONLY=true
    if [ "$is_pi" = true ]; then
        warn "Driver setup failed ($step) — continuing in PREVIEW-ONLY mode (--allow-preview-only)"
    else
        info "Not a Raspberry Pi — continuing in preview-only mode ($step)"
    fi
}

install_gpio_packages() {
    local pkg
    FAILED_GPIO_PACKAGES=""
    info "Installing GPIO packages..."
    for pkg in "${GPIO_PACKAGES[@]}"; do
        if ! run "$VENV_DIR/bin/pip" install --no-cache-dir "$pkg"; then
            warn "GPIO package failed to install: $pkg"
            FAILED_GPIO_PACKAGES="$FAILED_GPIO_PACKAGES $pkg"
        fi
    done
}

# Idempotency guard ($1 = marker file): 0 = installed pin matches.
waveshare_pin_current() {
    [ -f "$1" ] && [ "$(cat "$1" 2>/dev/null)" = "$WAVESHARE_EPD_PIN" ]
}

# Importing the driver constructs epdconfig's gpiozero objects at import time,
# so the pin factory must be forced here too — otherwise gpiozero auto-selects
# the broken 'native' factory on kernel >= 6.6 and the check fails spuriously.
driver_import_ok() {
    GPIOZERO_PIN_FACTORY="${PIN_FACTORY:-$PIN_FACTORY_DEFAULT}" \
        "$VENV_DIR/bin/python3" -c "from waveshare_epd import ${EINK_DISPLAY_DRIVER:-epd7in3e}" >/dev/null 2>&1
}

# 0 = the venv python can import lgpio (via the apt python3-lgpio in the
# system dist-packages, exposed through the --system-site-packages venv).
lgpio_import_ok() {
    "$VENV_DIR/bin/python3" -c "import lgpio" >/dev/null 2>&1
}

# 0 = 'import RPi.GPIO' works AND Jetson.GPIO is no longer installed — the
# Jetson shim cannot function without it, so this confirms the genuine RPi.GPIO
# resolves after de-shadowing.
rpi_gpio_is_genuine() {
    "$VENV_DIR/bin/python3" -c "import RPi.GPIO" >/dev/null 2>&1 \
        && ! "$VENV_DIR/bin/pip" show "$JETSON_GPIO_PACKAGE" >/dev/null 2>&1
}

# Makes lgpio importable in the venv. On current Raspberry Pi OS (Trixie) the
# distro package python3-lgpio does NOT exist and lgpio ships no wheel for
# Python 3.13, so the swig SOURCE BUILD is the primary path on the real target;
# the apt attempt is kept first only because it is harmless and works on the
# OSes that do ship it.
#
# INFORMATIONAL by contract: this function NEVER aborts and ALWAYS returns 0 —
# it does not decide preview-only. select_pin_factory re-checks lgpio as the
# authoritative signal and check_driver_import owns the ALLOW_PREVIEW_ONLY
# policy. (Earlier this ended in a bare `lgpio_import_ok`, so a non-zero return
# under `set -euo pipefail` aborted the whole script before the fallback / gate
# / preview-only logic could run — see E2.5a review.)
install_lgpio() {
    info "Installing lgpio pin factory..."
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] sudo apt-get install -y $LGPIO_APT_PACKAGE (best effort; absent on Debian Trixie)"
        echo "[DRY-RUN] sudo apt-get install -y swig (build dep for the lgpio source build)"
        echo "[DRY-RUN] $VENV_DIR/bin/pip install lgpio>=0.2 (source build — no distro package/wheel for this Python)"
        echo "[DRY-RUN] $VENV_DIR/bin/python3 -c 'import lgpio' (verify importable)"
        return 0
    fi
    # 1) Distro package first (harmless where present; just warn when absent).
    if run sudo apt-get install -y -qq "$LGPIO_APT_PACKAGE" && lgpio_import_ok; then
        info "lgpio available via $LGPIO_APT_PACKAGE"
        return 0
    fi
    # 2) Source build (the real primary path on Trixie/Py3.13). swig is the
    #    build dep the pip build needs (HIL-3); gcc/python3-dev are already
    #    installed by install_system_packages. Install swig BEFORE pip build.
    info "Building lgpio from source (no distro package/wheel for this Python)..."
    if ! run sudo apt-get install -y -qq swig; then
        warn "swig install failed — the lgpio source build will likely fail"
    fi
    if ! "$VENV_DIR/bin/pip" install --no-cache-dir "lgpio>=0.2"; then
        warn "lgpio source build failed"
    fi
    if lgpio_import_ok; then
        info "lgpio built and importable"
    else
        warn "lgpio still not importable — pin-factory selection / the driver gate will decide next"
    fi
    return 0
}

# Removes the Jetson.GPIO contamination and restores the genuine RPi.GPIO
# (uninstalling Jetson.GPIO can take the co-located RPi/GPIO files with it).
remove_jetson_gpio() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] $VENV_DIR/bin/pip uninstall -y $JETSON_GPIO_PACKAGE (its RPi/GPIO shim shadows real RPi.GPIO)"
        echo "[DRY-RUN] $VENV_DIR/bin/pip install --no-cache-dir --force-reinstall --no-deps '$RPI_GPIO_CONSTRAINT' (restore genuine RPi.GPIO)"
        return 0
    fi
    if "$VENV_DIR/bin/pip" show "$JETSON_GPIO_PACKAGE" >/dev/null 2>&1; then
        "$VENV_DIR/bin/pip" uninstall -y "$JETSON_GPIO_PACKAGE" || warn "could not uninstall $JETSON_GPIO_PACKAGE"
        "$VENV_DIR/bin/pip" install --no-cache-dir --force-reinstall --no-deps "$RPI_GPIO_CONSTRAINT" \
            || warn "could not reinstall genuine RPi.GPIO"
        info "Removed $JETSON_GPIO_PACKAGE and restored genuine RPi.GPIO"
    fi
    if ! rpi_gpio_is_genuine; then
        warn "RPi.GPIO not importable/genuine after $JETSON_GPIO_PACKAGE removal"
    fi
}

# Pure decision ($1 = lgpio_ok, $2 = kernel_ge_66, both "true"/"false"):
# echoes the pin factory to use, or "fatal" when lgpio is missing on kernel
# >= 6.6, where there is no working fallback.
decide_pin_factory() {
    if [ "$1" = "true" ]; then
        echo "$PIN_FACTORY_DEFAULT"
    elif [ "$2" = "true" ]; then
        echo "fatal"
    else
        echo "$PIN_FACTORY_FALLBACK"
    fi
}

# Sets PIN_FACTORY from lgpio availability + kernel version. On a fatal outcome
# (lgpio missing, kernel >= 6.6) it keeps lgpio so the import gate genuinely
# tries — and fails loudly — rather than silently using a broken factory.
select_pin_factory() {
    local lgpio_ok=false kernel_modern=false decision
    if [ "$DRY_RUN" = true ] || lgpio_import_ok; then
        lgpio_ok=true
    fi
    if kernel_ge_66 "$(kernel_release)"; then
        kernel_modern=true
    fi
    decision="$(decide_pin_factory "$lgpio_ok" "$kernel_modern")"
    if [ "$decision" = "fatal" ]; then
        FAILED_GPIO_PACKAGES="$FAILED_GPIO_PACKAGES $LGPIO_APT_PACKAGE"
        PIN_FACTORY="$PIN_FACTORY_DEFAULT"
        warn "lgpio unavailable on kernel >= 6.6 — no working pin factory; the driver gate will refuse (see E2.5a)"
        return 0
    fi
    if [ "$lgpio_ok" != true ]; then
        FAILED_GPIO_PACKAGES="$FAILED_GPIO_PACKAGES $LGPIO_APT_PACKAGE"
        warn "lgpio unavailable — falling back to pin factory '$decision' (kernel < 6.6, documented)"
    fi
    PIN_FACTORY="$decision"
    info "Pin factory: $PIN_FACTORY"
}

# Lean pinned fetch: partial clone of just the python subdir instead of a
# pip VCS install of the huge upstream repo at HEAD (SD/RAM-friendly, reproducible).
fetch_waveshare_source() {
    local tmp="$1"
    git -C "$tmp" init -q || return 1
    git -C "$tmp" remote add origin "$WAVESHARE_EPD_REPO" || return 1
    # Probe partial fetch quietly first; the fallback below is loud on failure.
    if ! git -C "$tmp" fetch --depth 1 --filter=blob:none origin "$WAVESHARE_EPD_PIN" 2>/dev/null; then
        warn "Partial fetch not supported here — falling back to a plain depth-1 fetch"
        git -C "$tmp" fetch --depth 1 origin "$WAVESHARE_EPD_PIN" || return 1
    fi
    # Best effort: restricts checkout to the python subdir (old git: full checkout).
    git -C "$tmp" sparse-checkout set "$WAVESHARE_EPD_SUBDIR" 2>/dev/null || true
    git -C "$tmp" checkout -q FETCH_HEAD || return 1
}

install_waveshare_driver() {
    local marker="$VENV_DIR/.waveshare_pin" tmp
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] fetch Waveshare e-Paper pin $WAVESHARE_EPD_PIN from $WAVESHARE_EPD_REPO (sparse: $WAVESHARE_EPD_SUBDIR)"
        echo "[DRY-RUN] $VENV_DIR/bin/pip install --no-cache-dir <tmp>/$WAVESHARE_EPD_SUBDIR"
        echo "[DRY-RUN] write pin marker $marker"
        return 0
    fi
    if waveshare_pin_current "$marker" && driver_import_ok; then
        info "Waveshare driver already at pin $WAVESHARE_EPD_PIN (skipped)"
        return 0
    fi
    info "Installing Waveshare e-Paper driver (pin $WAVESHARE_EPD_PIN)..."
    tmp="$(mktemp -d)"
    if ! fetch_waveshare_source "$tmp"; then
        rm -rf "$tmp"
        return 1
    fi
    if ! "$VENV_DIR/bin/pip" install --no-cache-dir "$tmp/$WAVESHARE_EPD_SUBDIR"; then
        rm -rf "$tmp"
        return 1
    fi
    rm -rf "$tmp"
    printf '%s' "$WAVESHARE_EPD_PIN" > "$marker"
}

check_driver_import() {
    local driver="${EINK_DISPLAY_DRIVER:-epd7in3e}"
    local factory="${PIN_FACTORY:-$PIN_FACTORY_DEFAULT}"
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] GPIOZERO_PIN_FACTORY=$factory $VENV_DIR/bin/python3 -c 'from waveshare_epd import $driver' (final import check)"
        return 0
    fi
    if [ "$PREVIEW_ONLY" = true ]; then
        return 0
    fi
    if driver_import_ok; then
        info "Driver import check passed (waveshare_epd.$driver, pin factory: $factory)"
        return 0
    fi
    local detail="final import check 'from waveshare_epd import $driver' (pin factory: $factory)"
    if [ -n "$FAILED_GPIO_PACKAGES" ]; then
        detail="$detail; failed GPIO packages:$FAILED_GPIO_PACKAGES"
    fi
    detail="$detail; on kernel >= 6.6 lgpio ($LGPIO_APT_PACKAGE) is the only working gpiozero factory — see E2.5a"
    apply_driver_policy "$detail"
}

install_display_stack() {
    if ! is_raspberry_pi; then
        PREVIEW_ONLY=true
        info "Not a Raspberry Pi — display driver stack skipped, client will run in PREVIEW-ONLY mode"
        return 0
    fi
    # install_lgpio is informational and returns 0; `|| true` is belt-and-braces
    # so a future edit reintroducing a non-zero return can never abort the
    # script under `set -euo pipefail` before the fallback / gate / preview-only
    # logic below runs.
    install_lgpio || true
    install_gpio_packages
    if ! install_waveshare_driver; then
        apply_driver_policy "Waveshare driver installation (pin $WAVESHARE_EPD_PIN)"
        return 0
    fi
    remove_jetson_gpio
    select_pin_factory
    check_driver_import
}

# ----- Data dirs, .env, logrotate -----
create_data_dirs() {
    run mkdir -p \
        "$SCRIPT_DIR/data/designs/history" \
        "$SCRIPT_DIR/data/uploaded_images/thumbs" \
        "$SCRIPT_DIR/data/fonts" \
        "$SCRIPT_DIR/data/weather_styles" \
        "$SCRIPT_DIR/logs"
}

create_env_file() {
    if [ -f "$SCRIPT_DIR/.env" ]; then
        info ".env already exists (kept unchanged)"
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] create .env from .env.example (DATA_DIR=./data)"
        return 0
    fi
    sed 's|DATA_DIR=/app/data|DATA_DIR=./data|' "$SCRIPT_DIR/.env.example" > "$SCRIPT_DIR/.env"
    info ".env created — edit to customize settings"
}

# ----- GPIOZERO_PIN_FACTORY (E2.5a) -----
# 0 = env file ($1) has no active GPIOZERO_PIN_FACTORY value yet (a commented
# example line in .env.example does not count).
pin_factory_env_needed() {
    ! grep -Eq '^GPIOZERO_PIN_FACTORY=.+' "$1" 2>/dev/null
}

# Idempotent: writes GPIOZERO_PIN_FACTORY=$2 into env file $1 (fills an empty
# line if present, else appends). An existing non-empty value is never
# overwritten, so a manual user override wins.
set_pin_factory_env() {
    local envfile="$1" factory="$2" tmp
    if ! pin_factory_env_needed "$envfile"; then
        return 0
    fi
    if grep -q '^GPIOZERO_PIN_FACTORY=' "$envfile" 2>/dev/null; then
        tmp="$(mktemp)"
        sed "s/^GPIOZERO_PIN_FACTORY=[[:space:]]*$/GPIOZERO_PIN_FACTORY=$factory/" "$envfile" > "$tmp"
        mv "$tmp" "$envfile"
    else
        printf 'GPIOZERO_PIN_FACTORY=%s\n' "$factory" >> "$envfile"
    fi
}

# Persists the selected pin factory into .env (both systemd units and the
# manual ./eink.sh path read it). Skipped in preview-only mode (no panel, so
# no factory is forced and gpiozero auto-detection stays out of the way).
configure_pin_factory_env() {
    local envfile="${1:-$SCRIPT_DIR/.env}" factory="${PIN_FACTORY:-$PIN_FACTORY_DEFAULT}"
    if [ "$PREVIEW_ONLY" = true ]; then
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] set GPIOZERO_PIN_FACTORY=$factory in .env (client pin factory; lgpio on kernel >= 6.6)"
        return 0
    fi
    if pin_factory_env_needed "$envfile"; then
        set_pin_factory_env "$envfile" "$factory"
        info "Pin factory pinned in .env: GPIOZERO_PIN_FACTORY=$factory"
    else
        info "GPIOZERO_PIN_FACTORY already set in .env (kept)"
    fi
}

# ----- EINK_CLIENT_TOKEN (E5.1 authentication) -----
# 0 = env file ($1) needs a token: file/line missing or empty value.
client_token_needed() {
    ! grep -Eq '^EINK_CLIENT_TOKEN=.+' "$1" 2>/dev/null
}

generate_client_token() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
    else
        head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
    fi
}

# Writes token $2 into env file $1: fills an empty EINK_CLIENT_TOKEN= line
# (fresh .env from .env.example) or appends the variable (pre-auth .env).
# A non-empty existing value is NEVER overwritten.
set_client_token() {
    local envfile="$1" token="$2" tmp
    if ! client_token_needed "$envfile"; then
        return 0
    fi
    if grep -q '^EINK_CLIENT_TOKEN=' "$envfile" 2>/dev/null; then
        tmp="$(mktemp)"
        sed "s/^EINK_CLIENT_TOKEN=[[:space:]]*$/EINK_CLIENT_TOKEN=$token/" "$envfile" > "$tmp"
        mv "$tmp" "$envfile"
    else
        printf 'EINK_CLIENT_TOKEN=%s\n' "$token" >> "$envfile"
    fi
    # The file carries a secret now; both systemd units read it as EnvironmentFile.
    chmod 600 "$envfile"
}

# Idempotent: ensures .env carries a generated EINK_CLIENT_TOKEN (both
# systemd units load the same .env, so server and client share it). The
# token value is never printed or logged — not even in dry-run plans.
ensure_client_token() {
    local envfile="${1:-$SCRIPT_DIR/.env}"
    if [ "$DRY_RUN" = true ]; then
        if client_token_needed "$envfile"; then
            echo "[DRY-RUN] generate EINK_CLIENT_TOKEN into .env (openssl rand -hex 32; value not printed)"
        else
            info "EINK_CLIENT_TOKEN already set in .env (kept)"
        fi
        return 0
    fi
    if ! client_token_needed "$envfile"; then
        info "EINK_CLIENT_TOKEN already set in .env (kept)"
        return 0
    fi
    set_client_token "$envfile" "$(generate_client_token)"
    info "EINK_CLIENT_TOKEN generated into .env (shared by server and client)"
}

install_logrotate() {
    if [ "$DRY_RUN" = true ]; then
        echo "[DRY-RUN] write /etc/logrotate.d/eink-picture (rotate $SCRIPT_DIR/logs/*.log)"
        return 0
    fi
    if ! command -v logrotate >/dev/null 2>&1; then
        warn "logrotate not installed — skipping log rotation config"
        return 0
    fi
    sudo tee /etc/logrotate.d/eink-picture > /dev/null <<EOF
$SCRIPT_DIR/logs/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
    size 10M
}
EOF
    info "Log rotation configured (/etc/logrotate.d/eink-picture)"
}

# ----- systemd (single source: templates in systemd/) -----
# Renders a unit template ($1) replacing %USER% ($2) and %INSTALL_DIR% ($3).
render_unit_template() {
    sed -e "s|%USER%|$2|g" -e "s|%INSTALL_DIR%|$3|g" "$1"
}

install_systemd_services() {
    if ! confirm "Install systemd services (autostart on boot)? [Y/n]" y; then
        info "Skipping systemd services — manual mode: ./eink.sh start"
        return 0
    fi
    local unit service_user
    service_user="$(id -un)"
    for unit in "${SYSTEMD_UNITS[@]}"; do
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] render systemd/$unit -> /etc/systemd/system/$unit (USER=$service_user, INSTALL_DIR=$SCRIPT_DIR)"
        else
            render_unit_template "$SCRIPT_DIR/systemd/$unit" "$service_user" "$SCRIPT_DIR" \
                | sudo tee /etc/systemd/system/"$unit" > /dev/null
        fi
    done
    run sudo systemctl daemon-reload
    run sudo systemctl enable --now "${SYSTEMD_UNITS[@]}"
    if [ "$UPDATE_MODE" = true ]; then
        run sudo systemctl restart "${SYSTEMD_UNITS[@]}"
    fi
    info "Systemd services installed and started (enable --now)"
}

# ----- Summary -----
print_summary() {
    local ip
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    echo ""
    echo "=========================================="
    if [ "$DRY_RUN" = true ]; then
        echo "  Dry Run Complete — nothing was changed"
    elif [ "$UPDATE_MODE" = true ]; then
        echo "  Update Complete!"
    else
        echo "  Setup Complete!"
    fi
    echo "=========================================="
    echo ""
    echo "  Services:  sudo systemctl status eink-server eink-client"
    echo "  Manual:    ./eink.sh start|stop|status|logs"
    echo ""
    echo "  Designer:  http://${ip:-localhost}:${PORT:-5000}/designer"
    echo "  Health:    http://${ip:-localhost}:${PORT:-5000}/health"
    if [ "$PREVIEW_ONLY" = true ]; then
        echo ""
        echo "  NOTE: PREVIEW-ONLY mode — no e-paper driver is active."
        echo "        The client will fetch previews but cannot drive a display."
    fi
    if [ "$REBOOT_REQUIRED" = true ]; then
        echo ""
        echo "  IMPORTANT: SPI was just enabled — a REBOOT is required (sudo reboot)."
        echo "             Services start again automatically after reboot (Restart=always)."
    fi
    echo ""
}

# ----- Main -----
main() {
    parse_flags "$@"
    auto_headless
    cd "$SCRIPT_DIR"
    export PATH="/usr/local/go/bin:$PATH"

    echo "=========================================="
    echo "  E-Ink Picture — Native Pi Setup"
    echo "=========================================="
    if [ "$DRY_RUN" = true ]; then
        echo "  (dry run: printing planned actions only)"
    fi
    echo ""

    setup_platform
    preflight
    update_checkout
    install_system_packages
    enable_spi
    ensure_go
    build_server
    setup_venv
    install_core_python
    install_display_stack
    create_data_dirs
    create_env_file
    configure_pin_factory_env
    ensure_client_token
    install_logrotate
    run chmod +x "$SCRIPT_DIR/eink.sh"
    install_systemd_services
    print_summary
}

if [ "$EINK_SOURCED" != true ]; then
    main "$@"
fi
