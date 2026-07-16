#!/bin/bash
# End-to-End tests for E-Ink Picture
#
# Two phases (specs/E5.1-authentication.md AC9):
#   Phase A — fresh DATA_DIR, no password set: legacy checks, everything open.
#   Phase B — password set via /api/auth/setup: 401 without cookie, login via
#             cookie jar, client endpoints via X-Client-Token, logout.
#
# Default mode (no EINK_SERVER_URL): builds and starts its own server on
# EINK_E2E_PORT (default 5077) with a fresh DATA_DIR (or $DATA_DIR if set)
# and a generated EINK_CLIENT_TOKEN, and shuts it down afterwards.
#
# External mode (EINK_SERVER_URL set): runs Phase A against the given server,
# which must NOT have a password set. Phase B is skipped unless
# EINK_E2E_PHASE_B=1 (it permanently sets a password on that server!);
# the token checks then need EINK_CLIENT_TOKEN to match the server's.
set -e

PASS=0
FAIL=0

check() {
    local name="$1"
    local result="$2"
    if [ "$result" = "0" ]; then
        echo "  PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $name"
        FAIL=$((FAIL + 1))
    fi
}

# check_code NAME EXPECTED_HTTP_CODE curl-args...
check_code() {
    local name="$1"
    local want="$2"
    shift 2
    local got
    got=$(curl -s -o /dev/null -w "%{http_code}" "$@")
    if [ "$got" = "$want" ]; then
        echo "  PASS: $name ($got)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $name (got $got, want $want)"
        FAIL=$((FAIL + 1))
    fi
}

MANAGED=0
if [ -z "$EINK_SERVER_URL" ]; then
    MANAGED=1
    ROOT="$(cd "$(dirname "$0")/.." && pwd)"
    PORT="${EINK_E2E_PORT:-5077}"
    SERVER="http://localhost:$PORT"
    DATA_DIR="${DATA_DIR:-$(mktemp -d)}"
    EINK_CLIENT_TOKEN="${EINK_CLIENT_TOKEN:-$(openssl rand -hex 32)}"
    E2E_TMP="$(mktemp -d)"

    echo "Building server..."
    (cd "$ROOT/server" && go build -o "$E2E_TMP/eink-server" .)

    PORT="$PORT" DATA_DIR="$DATA_DIR" DEPLOYMENT_MODE=local \
        EINK_CLIENT_TOKEN="$EINK_CLIENT_TOKEN" \
        "$E2E_TMP/eink-server" >"$E2E_TMP/server.log" 2>&1 &
    SERVER_PID=$!

    cleanup() {
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
        rm -rf "$E2E_TMP"
    }
    trap cleanup EXIT

    for _ in $(seq 1 50); do
        if curl -sf "$SERVER/health" >/dev/null 2>&1; then
            break
        fi
        sleep 0.2
    done
else
    SERVER="$EINK_SERVER_URL"
fi

echo "=== E-Ink Picture E2E Tests ==="
echo "Server: $SERVER"
echo ""

# Guard: Phase A requires an instance without a password.
if curl -sf "$SERVER/api/auth/status" 2>/dev/null | grep -q '"password_set":true'; then
    echo "ERROR: server already has a password set — Phase A needs a fresh DATA_DIR"
    exit 1
fi

echo "=== Phase A: no password set (legacy behavior) ==="

# Health
echo "[Health]"
curl -sf "$SERVER/health" | grep -q "ok" 2>/dev/null
check "GET /health" $?

# Auth status
echo "[Auth Status]"
curl -sf "$SERVER/api/auth/status" | grep -q '"password_set":false' 2>/dev/null
check "GET /api/auth/status — password_set false" $?

# Designer HTML
echo "[Designer]"
curl -sf "$SERVER/designer" | grep -qi "canvas\|fabric\|designer" 2>/dev/null
check "GET /designer" $?

# Login page (public)
curl -sf "$SERVER/login" | grep -qi "password" 2>/dev/null
check "GET /login" $?

# Settings
echo "[Settings]"
SETTINGS=$(curl -sf "$SERVER/settings" 2>/dev/null)
echo "$SETTINGS" | grep -q "display_type" 2>/dev/null
check "GET /settings — has display_type" $?
echo "$SETTINGS" | grep -q "refresh_interval" 2>/dev/null
check "GET /settings — has refresh_interval" $?

# Display Profiles
echo "[Display Profiles]"
curl -sf "$SERVER/display_profiles" | grep -q "waveshare" 2>/dev/null
check "GET /display_profiles" $?

# Designs
echo "[Designs]"
curl -sf "$SERVER/designs" >/dev/null 2>&1
check "GET /designs" $?

# Preview PNG
echo "[Preview]"
curl -sf "$SERVER/preview" -o /tmp/eink_preview_test.png 2>/dev/null
file /tmp/eink_preview_test.png 2>/dev/null | grep -q "PNG" 2>/dev/null
check "GET /preview — is PNG" $?

if command -v python3 &>/dev/null && python3 -c "from PIL import Image" 2>/dev/null; then
    python3 -c "
from PIL import Image
img = Image.open('/tmp/eink_preview_test.png')
assert img.size == (800, 480), f'Wrong size: {img.size}'
print(f'  INFO: Preview {img.size[0]}x{img.size[1]}, mode={img.mode}')
" 2>/dev/null
    check "Preview size 800x480" $?
else
    echo "  SKIP: Preview size check (PIL not available)"
fi

# Refresh API
echo "[Refresh API]"
TRIGGER=$(curl -sf -X POST "$SERVER/api/trigger_refresh" 2>/dev/null)
echo "$TRIGGER" | grep -q "refresh_triggered\|triggered" 2>/dev/null
check "POST /api/trigger_refresh" $?

STATUS=$(curl -sf "$SERVER/api/refresh_status" 2>/dev/null)
echo "$STATUS" | grep -q "should_refresh" 2>/dev/null
check "GET /api/refresh_status — has should_refresh" $?
echo "$STATUS" | grep -q "refresh_interval" 2>/dev/null
check "GET /api/refresh_status — has refresh_interval" $?

HEARTBEAT=$(curl -sf -X POST "$SERVER/api/client_heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"status":"refreshed","timestamp":"2026-03-06T14:30:00Z"}' 2>/dev/null)
echo "$HEARTBEAT" | grep -q "true\|ok" 2>/dev/null
check "POST /api/client_heartbeat" $?

# Update settings with refresh interval
echo "[Settings Update]"
UPDATE=$(curl -sf -X POST "$SERVER/update_settings" \
    -H "Content-Type: application/json" \
    -d '{"refresh_interval": 1800}' 2>/dev/null)
echo "$UPDATE" | grep -q "1800" 2>/dev/null
check "POST /update_settings — refresh_interval" $?

# Fonts & Images
echo "[Media]"
curl -sf "$SERVER/fonts_all" >/dev/null 2>&1
check "GET /fonts_all" $?
curl -sf "$SERVER/images_all" >/dev/null 2>&1
check "GET /images_all" $?

# Widget APIs
echo "[Widget APIs]"
curl -sf "$SERVER/api/widget_layouts/widget_system" >/dev/null 2>&1
check "GET /api/widget_layouts/widget_system" $?

# Static files
echo "[Static]"
curl -sf "$SERVER/static/js/designer.js" | grep -q "Designer" 2>/dev/null
check "GET /static/js/designer.js" $?

# --- Phase B ---
if [ "$MANAGED" = "1" ] || [ "$EINK_E2E_PHASE_B" = "1" ]; then
    echo ""
    echo "=== Phase B: authentication enabled ==="
    E2E_PW="e2e-test-password"
    JAR="${E2E_TMP:-/tmp}/e2e_cookies.txt"
    rm -f "$JAR"

    echo "[Setup]"
    check_code "POST /api/auth/setup" 200 \
        -X POST "$SERVER/api/auth/setup" \
        -H "Content-Type: application/json" -d "{\"password\":\"$E2E_PW\"}"

    echo "[Guard]"
    check_code "GET /designs without cookie — 401" 401 "$SERVER/designs"
    check_code "GET /designer without cookie — 302 to /login" 302 "$SERVER/designer"
    check_code "GET /health stays public" 200 "$SERVER/health"
    check_code "GET /api/does_not_exist — 401 (deny-by-default)" 401 "$SERVER/api/does_not_exist"

    echo "[Login]"
    check_code "POST /api/auth/login — 200 + cookie" 200 \
        -c "$JAR" -X POST "$SERVER/api/auth/login" \
        -H "Content-Type: application/json" -d "{\"password\":\"$E2E_PW\"}"
    check_code "GET /designs with cookie — 200" 200 -b "$JAR" "$SERVER/designs"

    echo "[Client Token]"
    if [ -n "$EINK_CLIENT_TOKEN" ]; then
        check_code "GET /api/refresh_status with X-Client-Token — 200" 200 \
            -H "X-Client-Token: $EINK_CLIENT_TOKEN" "$SERVER/api/refresh_status"
        check_code "GET /settings with X-Client-Token — 200" 200 \
            -H "X-Client-Token: $EINK_CLIENT_TOKEN" "$SERVER/settings"
        check_code "POST /update_settings with token only — 401 (no general key)" 401 \
            -X POST -H "X-Client-Token: $EINK_CLIENT_TOKEN" \
            -H "Content-Type: application/json" -d '{"refresh_interval":1800}' \
            "$SERVER/update_settings"
    else
        echo "  SKIP: client token checks (EINK_CLIENT_TOKEN not set)"
    fi
    check_code "GET /api/refresh_status without token — 401" 401 "$SERVER/api/refresh_status"

    echo "[Logout]"
    check_code "POST /api/auth/logout — 200" 200 \
        -b "$JAR" -X POST "$SERVER/api/auth/logout"
    check_code "GET /designs after logout — 401" 401 -b "$JAR" "$SERVER/designs"
else
    echo ""
    echo "SKIP: Phase B (external server; set EINK_E2E_PHASE_B=1 to run — sets a permanent password!)"
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
