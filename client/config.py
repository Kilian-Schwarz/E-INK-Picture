"""E-Ink Picture Client Configuration."""
import os

SERVER_URL = os.getenv("EINK_SERVER_URL", "http://localhost:5000")
DISPLAY_WIDTH = int(os.getenv("EINK_WIDTH", "800"))
DISPLAY_HEIGHT = int(os.getenv("EINK_HEIGHT", "480"))
REFRESH_INTERVAL = int(os.getenv("EINK_REFRESH_INTERVAL", "300"))
DEPLOYMENT_MODE = os.getenv("EINK_DEPLOYMENT_MODE", "local")
OFFSET_X = int(os.getenv("EINK_OFFSET_X", "0"))
OFFSET_Y = int(os.getenv("EINK_OFFSET_Y", "0"))
LOG_LEVEL = os.getenv("EINK_LOG_LEVEL", "INFO")
