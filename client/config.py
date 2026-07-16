"""E-Ink Picture Client Configuration."""
import os

SERVER_URL = os.getenv("EINK_SERVER_URL", "http://localhost:5000")
DISPLAY_DRIVER = os.getenv("EINK_DISPLAY_DRIVER", "epd7in3e")
REFRESH_INTERVAL = int(os.getenv("EINK_REFRESH_INTERVAL", "3600"))
POLL_INTERVAL = int(os.getenv("EINK_POLL_INTERVAL", "30"))
# Read timeout (seconds) for the long-polling /api/refresh_status request.
# Must be strictly greater than the server hold (25s) so the server always
# responds before the client's read times out; otherwise the client keeps
# reconnecting needlessly (no missed trigger, just churn).
LONGPOLL_TIMEOUT = int(os.getenv("EINK_LONGPOLL_TIMEOUT", "30"))
DEPLOYMENT_MODE = os.getenv("EINK_DEPLOYMENT_MODE", "local")
LOG_LEVEL = os.getenv("EINK_LOG_LEVEL", "INFO")
LAST_SENT_PATH = os.getenv("EINK_LAST_SENT_PATH", "/tmp/eink_last_sent.png")
CLIENT_TOKEN = os.getenv("EINK_CLIENT_TOKEN", "")
# Content skip (E5.2): default enabled; only the literal string "false"
# (case-insensitive) disables it.
CONTENT_SKIP = os.getenv("EINK_CONTENT_SKIP", "").lower() != "false"
# Panel care guard: force a panel write after this many hours even if the
# content is unchanged (Waveshare: at least 1 refresh per 24h). 0 = off.
MAX_SKIP_HOURS = int(os.getenv("EINK_MAX_SKIP_HOURS", "24"))
# Watchdog escalation (E5.4): exit with a non-zero code after this many
# consecutive hardware failure cycles so systemd restarts the process with a
# freshly imported driver stack. 0 = never escalate (per-cycle recovery only).
HW_FAILURE_LIMIT = int(os.getenv("EINK_HW_FAILURE_LIMIT", "3"))
