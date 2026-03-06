"""E-Ink Picture Client Configuration."""
import os

SERVER_URL = os.getenv("EINK_SERVER_URL", "http://localhost:5000")
DISPLAY_DRIVER = os.getenv("EINK_DISPLAY_DRIVER", "epd7in3e")
REFRESH_INTERVAL = int(os.getenv("EINK_REFRESH_INTERVAL", "3600"))
POLL_INTERVAL = int(os.getenv("EINK_POLL_INTERVAL", "30"))
DEPLOYMENT_MODE = os.getenv("EINK_DEPLOYMENT_MODE", "local")
LOG_LEVEL = os.getenv("EINK_LOG_LEVEL", "INFO")
