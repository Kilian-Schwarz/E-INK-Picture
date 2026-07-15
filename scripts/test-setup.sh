#!/bin/bash
# Unit-style tests for the detection/decision functions in setup.sh.
# bash 3.2 compatible (runs on macOS /bin/bash) — no associative arrays,
# no ${var,,}. setup.sh is sourced via its source guard: main() does not run.
#
# Usage: bash scripts/test-setup.sh
#
# shellcheck disable=SC2329  # test functions/helpers are invoked indirectly via run_test
set -u

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$TEST_DIR/.." && pwd)"

# shellcheck source=setup.sh
source "$REPO_DIR/setup.sh"

# setup.sh enables errexit for its own runs; assertions below manage
# failures explicitly, so switch it off for the test harness.
set +e

TESTS_RUN=0
TESTS_PASSED=0
CURRENT_TEST=""
FAILED_IN_TEST=0

fail() {
    echo "    FAIL: $CURRENT_TEST: ${1:-}"
    FAILED_IN_TEST=1
}

assert_eq() {
    # $1 expected, $2 actual, $3 label
    if [ "$1" != "$2" ]; then
        fail "$3: expected '$1', got '$2'"
    fi
}

run_test() {
    CURRENT_TEST="$1"
    FAILED_IN_TEST=0
    TESTS_RUN=$((TESTS_RUN + 1))
    "$1"
    if [ "$FAILED_IN_TEST" -eq 0 ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo "PASS: $1"
    else
        echo "FAIL: $1"
    fi
}

# ----- Tests -----

test_arch_mapping() {
    assert_eq "arm64 arm64 -" "$(map_architecture aarch64)" "aarch64 mapping"
    assert_eq "arm armv6l 7" "$(map_architecture armv7l)" "armv7l mapping (armv6l tarball)"
    assert_eq "arm armv6l 6" "$(map_architecture armv6l)" "armv6l mapping"
    assert_eq "amd64 amd64 -" "$(map_architecture x86_64)" "x86_64 mapping"
    if map_architecture mips >/dev/null 2>&1; then
        fail "unsupported arch 'mips' must be rejected"
    fi
    assert_eq "go${GO_VERSION}.linux-arm64.tar.gz" "$(go_tarball_name arm64)" "arm64 tarball name"
    assert_eq "go${GO_VERSION}.linux-armv6l.tar.gz" "$(go_tarball_name armv6l)" "armv6l tarball name"
    assert_eq "go${GO_VERSION}.linux-amd64.tar.gz" "$(go_tarball_name amd64)" "amd64 tarball name"
}

test_pi_detection() {
    EINK_TEST_PI_MODEL="Raspberry Pi Zero 2 W Rev 1.0"
    if ! is_raspberry_pi; then
        fail "injected Pi model must be detected as Raspberry Pi"
    fi
    assert_eq "Raspberry Pi Zero 2 W Rev 1.0" "$(pi_model)" "injected model string"
    EINK_TEST_PI_MODEL="Generic x86 Workstation"
    if is_raspberry_pi; then
        fail "non-Pi model must not be detected as Raspberry Pi"
    fi
    unset EINK_TEST_PI_MODEL
}

test_flag_parsing() {
    parse_flags --headless --update --allow-preview-only --dry-run
    assert_eq true "$HEADLESS" "--headless sets HEADLESS"
    assert_eq true "$UPDATE_MODE" "--update sets UPDATE_MODE"
    assert_eq true "$ALLOW_PREVIEW_ONLY" "--allow-preview-only sets ALLOW_PREVIEW_ONLY"
    assert_eq true "$DRY_RUN" "--dry-run sets DRY_RUN"

    parse_flags
    assert_eq false "$HEADLESS" "no flags resets HEADLESS"
    assert_eq false "$UPDATE_MODE" "no flags resets UPDATE_MODE"
    assert_eq false "$ALLOW_PREVIEW_ONLY" "no flags resets ALLOW_PREVIEW_ONLY"
    assert_eq false "$DRY_RUN" "no flags resets DRY_RUN"

    parse_flags --yes
    assert_eq true "$HEADLESS" "--yes is an alias for --headless"

    if (parse_flags --bogus) >/dev/null 2>&1; then
        fail "unknown flag must be rejected"
    fi

    # non-TTY stdin forces headless (curl | bash safety)
    parse_flags
    auto_headless < /dev/null >/dev/null
    assert_eq true "$HEADLESS" "non-TTY stdin forces headless"

    parse_flags # reset globals for subsequent tests
}

test_docker_preflight() {
    if ! docker_conflict "eink-picture-server ghcr.io/x/eink-picture:latest"; then
        fail "eink container name must be flagged as conflict"
    fi
    if ! docker_conflict "web e-ink-picture:local"; then
        fail "e-ink-picture image must be flagged as conflict"
    fi
    if docker_conflict "nginx nginx:latest
postgres postgres:16"; then
        fail "unrelated containers must not be flagged"
    fi
    if docker_conflict ""; then
        fail "missing docker / no containers must pass"
    fi

    # Full preflight with injected docker ps output (exit 1 on conflict).
    if (EINK_TEST_DOCKER_PS="eink-picture-server eink" preflight) >/dev/null 2>&1; then
        fail "preflight must abort when a project container is running"
    fi
    if ! (EINK_TEST_DOCKER_PS="" UPDATE_MODE=true preflight) >/dev/null 2>&1; then
        fail "preflight must pass without conflicts"
    fi
}

test_spi_config_fallback() {
    local tmp cfg
    tmp="$(mktemp -d)"
    mkdir -p "$tmp/boot/firmware"
    touch "$tmp/boot/firmware/config.txt" "$tmp/boot/config.txt"
    assert_eq "$tmp/boot/firmware/config.txt" "$(spi_config_path "$tmp")" "firmware path preferred (Bookworm+)"
    rm -rf "$tmp/boot/firmware"
    assert_eq "$tmp/boot/config.txt" "$(spi_config_path "$tmp")" "legacy /boot path fallback"

    cfg="$tmp/boot/config.txt"
    echo "# some comment" > "$cfg"
    ensure_spi_config_line "$cfg"
    assert_eq 1 "$(grep -c '^dtparam=spi=on' "$cfg")" "line added exactly once"
    ensure_spi_config_line "$cfg"
    assert_eq 1 "$(grep -c '^dtparam=spi=on' "$cfg")" "second run adds nothing"

    printf '#dtparam=spi=on\n' > "$cfg"
    ensure_spi_config_line "$cfg"
    if ! grep -q '^dtparam=spi=on' "$cfg"; then
        fail "commented-out line must not count as active"
    fi

    rm -rf "$tmp"
    if spi_config_path "$tmp" >/dev/null 2>&1; then
        fail "missing config.txt must return failure"
    fi
}

test_driver_fail_policy() {
    assert_eq "fatal" "$(decide_driver_failure true false)" "Pi + driver failure -> fatal"
    assert_eq "preview-only" "$(decide_driver_failure true true)" "Pi + failure + --allow-preview-only -> preview-only"
    assert_eq "preview-only" "$(decide_driver_failure false false)" "non-Pi + failure -> preview-only"
    assert_eq "preview-only" "$(decide_driver_failure false true)" "non-Pi + allow flag -> preview-only"
}

test_render_unit_template() {
    local unit out
    for unit in eink-server.service eink-client.service; do
        out="$(render_unit_template "$REPO_DIR/systemd/$unit" "pi" "/home/pi/E-INK-Picture")"
        case "$out" in
            *%USER%*|*%INSTALL_DIR%*)
                fail "$unit: unresolved placeholder after rendering" ;;
        esac
        if printf '%s\n' "$out" | grep -q '%'; then
            fail "$unit: leftover % placeholder after rendering"
        fi
        if ! printf '%s\n' "$out" | grep -q '^User=pi$'; then
            fail "$unit: %USER% not rendered to User=pi"
        fi
        if ! printf '%s\n' "$out" | grep -q '/home/pi/E-INK-Picture'; then
            fail "$unit: %INSTALL_DIR% not rendered"
        fi
        if ! printf '%s\n' "$out" | grep -q '^Restart=always$'; then
            fail "$unit: Restart=always missing"
        fi
        if ! printf '%s\n' "$out" | grep -q '^StartLimitIntervalSec=0$'; then
            fail "$unit: StartLimitIntervalSec=0 missing"
        fi
    done
}

test_idempotency_markers() {
    local tmp marker
    tmp="$(mktemp -d)"

    marker="$tmp/.waveshare_pin"
    if waveshare_pin_current "$marker"; then
        fail "missing pin marker must not allow skipping"
    fi
    printf '%s' "deadbeef" > "$marker"
    if waveshare_pin_current "$marker"; then
        fail "stale pin marker must not allow skipping"
    fi
    printf '%s' "$WAVESHARE_EPD_PIN" > "$marker"
    if ! waveshare_pin_current "$marker"; then
        fail "matching pin marker must allow skipping"
    fi

    marker="$tmp/.constraints"
    if ! core_python_needed "$marker"; then
        fail "missing constraints marker must trigger pip install"
    fi
    constraints_signature > "$marker"
    if core_python_needed "$marker"; then
        fail "matching constraints marker must skip pip install"
    fi
    printf '%s' "requests>=0,<1|Pillow>=0,<1" > "$marker"
    if ! core_python_needed "$marker"; then
        fail "changed constraints must trigger pip sync"
    fi

    rm -rf "$tmp"
}

test_client_token() {
    local tmp env out first
    tmp="$(mktemp -d)"
    env="$tmp/.env"

    # Missing file / missing line → token needed.
    if ! client_token_needed "$env"; then
        fail "missing env file must need a token"
    fi
    printf 'PORT=5000\n' > "$env"
    if ! client_token_needed "$env"; then
        fail "env without EINK_CLIENT_TOKEN line must need a token"
    fi

    # Generated tokens are 64 lowercase hex chars.
    local tok
    tok="$(generate_client_token)"
    assert_eq 64 "${#tok}" "generated token has 64 chars"
    case "$tok" in
        *[!0-9a-f]*) fail "generated token must be lowercase hex" ;;
    esac

    # Upgrade path: line missing → appended, other lines untouched.
    DRY_RUN=false
    ensure_client_token "$env" >/dev/null
    assert_eq 1 "$(grep -c '^EINK_CLIENT_TOKEN=' "$env")" "token line appended exactly once"
    if ! grep -Eq '^EINK_CLIENT_TOKEN=[0-9a-f]{64}$' "$env"; then
        fail "appended token must be 64 hex chars"
    fi
    if ! grep -q '^PORT=5000$' "$env"; then
        fail "existing lines must survive the append"
    fi

    # Idempotency: second run never overwrites the existing value.
    first="$(grep '^EINK_CLIENT_TOKEN=' "$env")"
    ensure_client_token "$env" >/dev/null
    assert_eq "$first" "$(grep '^EINK_CLIENT_TOKEN=' "$env")" "existing token never overwritten"

    # Fresh-install path: empty line from .env.example is filled in place.
    printf 'PORT=5000\nEINK_CLIENT_TOKEN=\nTZ=UTC\n' > "$env"
    ensure_client_token "$env" >/dev/null
    assert_eq 1 "$(grep -c '^EINK_CLIENT_TOKEN=' "$env")" "empty line filled, not duplicated"
    if ! grep -Eq '^EINK_CLIENT_TOKEN=[0-9a-f]{64}$' "$env"; then
        fail "empty line must be filled with a generated token"
    fi
    if ! grep -q '^TZ=UTC$' "$env"; then
        fail "lines after the token line must survive the fill"
    fi

    # Explicit never-overwrite check for the writer function itself.
    printf 'EINK_CLIENT_TOKEN=keep-this-value\n' > "$env"
    set_client_token "$env" "new-value"
    assert_eq "EINK_CLIENT_TOKEN=keep-this-value" "$(grep '^EINK_CLIENT_TOKEN=' "$env")" "set_client_token must not overwrite"

    # Dry run: plan only, no token value printed, file untouched.
    printf 'PORT=5000\n' > "$env"
    DRY_RUN=true
    out="$(ensure_client_token "$env")"
    case "$out" in
        *"[DRY-RUN]"*) : ;;
        *) fail "dry run must print a [DRY-RUN] plan line" ;;
    esac
    if printf '%s\n' "$out" | grep -Eq '[0-9a-f]{64}'; then
        fail "dry run must not print a token value (secrets hygiene)"
    fi
    if grep -q '^EINK_CLIENT_TOKEN=' "$env"; then
        fail "dry run must not modify the env file"
    fi
    DRY_RUN=false

    rm -rf "$tmp"
}

# ----- Runner -----
run_test test_arch_mapping
run_test test_pi_detection
run_test test_flag_parsing
run_test test_docker_preflight
run_test test_spi_config_fallback
run_test test_driver_fail_policy
run_test test_render_unit_template
run_test test_idempotency_markers
run_test test_client_token

echo ""
echo "${TESTS_PASSED}/${TESTS_RUN} tests passed"
if [ "$TESTS_PASSED" -ne "$TESTS_RUN" ]; then
    exit 1
fi
exit 0
