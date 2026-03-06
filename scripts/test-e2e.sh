#!/bin/bash
# End-to-End tests for E-Ink Picture
set -e

SERVER="${EINK_SERVER_URL:-http://localhost:5000}"
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

echo "=== E-Ink Picture E2E Tests ==="
echo "Server: $SERVER"
echo ""

# Health
echo "[Health]"
curl -sf "$SERVER/health" | grep -q "ok" 2>/dev/null
check "GET /health" $?

# Designer HTML
echo "[Designer]"
curl -sf "$SERVER/designer" | grep -qi "canvas\|fabric\|designer" 2>/dev/null
check "GET /designer" $?

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
curl -sf "$SERVER/api/widgets/system" >/dev/null 2>&1
check "GET /api/widgets/system" $?

# Static files
echo "[Static]"
curl -sf "$SERVER/static/js/designer.js" | grep -q "Designer" 2>/dev/null
check "GET /static/js/designer.js" $?

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
