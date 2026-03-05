#!/usr/bin/env python3
"""E-Ink Picture Client — fetches rendered preview from server and displays on E-Ink."""

import json
import logging
import signal
import sys
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from io import BytesIO
from typing import Optional

import requests
from PIL import Image

import config

logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL),
    format="%(asctime)s [%(levelname)s] %(message)s"
)
logger = logging.getLogger(__name__)

# Waveshare import (only available on Raspberry Pi)
try:
    from waveshare_epd import epd7in5_V2
    HAS_DISPLAY = True
except ImportError:
    logger.warning("Waveshare EPD library not found - running in preview mode")
    HAS_DISPLAY = False


class RefreshHandler(BaseHTTPRequestHandler):
    """HTTP handler for refresh trigger endpoint."""

    client: "EInkClient"

    def do_POST(self) -> None:
        if self.path == "/refresh":
            self.client.trigger_refresh()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok", "message": "Display refresh triggered"}).encode())
        else:
            self.send_error(404)

    def log_message(self, format: str, *args) -> None:
        logger.debug("RefreshServer: %s", format % args)


class EInkClient:
    """Client that fetches rendered images from server and displays on E-Ink."""

    def __init__(self) -> None:
        self.server_url = config.SERVER_URL.rstrip("/")
        self.timeout = 5 if config.DEPLOYMENT_MODE == "local" else 15
        self.running = True
        self.epd = None
        self._refresh_event = threading.Event()

    def fetch_preview(self) -> Optional[Image.Image]:
        """Fetch pre-rendered preview PNG from server."""
        url = f"{self.server_url}/preview"
        try:
            resp = requests.get(url, timeout=self.timeout)
            if resp.status_code == 200:
                return Image.open(BytesIO(resp.content))
            logger.error("Server returned status %d", resp.status_code)
        except requests.RequestException as e:
            logger.error("Failed to fetch preview: %s", e)
        return None

    def init_display(self) -> bool:
        """Initialize E-Ink display. Returns True if successful."""
        if not HAS_DISPLAY:
            logger.info("No display hardware - will save preview to file instead")
            return False
        try:
            self.epd = epd7in5_V2.EPD()
            self.epd.init()
            self.epd.Clear()
            logger.info("E-Ink display initialized")
            return True
        except Exception as e:
            logger.error("Failed to initialize display: %s", e)
            return False

    def display_image(self, img: Image.Image) -> None:
        """Send image to E-Ink display or save to file."""
        if self.epd:
            try:
                self.epd.init()
                self.epd.display(self.epd.getbuffer(img))
                self.epd.sleep()
                logger.info("Image displayed on E-Ink")
            except Exception as e:
                logger.error("Display error: %s", e)
        else:
            img.save("preview_output.png")
            logger.info("Preview saved to preview_output.png")

    def trigger_refresh(self) -> None:
        """Trigger an immediate display refresh."""
        logger.info("Manual refresh triggered")
        self._refresh_event.set()

    def start_refresh_server(self) -> None:
        """Start HTTP server for remote refresh triggers."""
        handler = type("Handler", (RefreshHandler,), {"client": self})
        server = HTTPServer(("0.0.0.0", config.CLIENT_PORT), handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        logger.info("Refresh server listening on port %d", config.CLIENT_PORT)

    def shutdown(self) -> None:
        """Clean shutdown - put display to sleep."""
        self.running = False
        if self.epd:
            try:
                self.epd.sleep()
                logger.info("Display in sleep mode")
            except Exception:
                pass
        if HAS_DISPLAY:
            try:
                epd7in5_V2.epdconfig.module_exit()
            except Exception:
                pass

    def run(self) -> None:
        """Main loop: fetch preview, display, wait, repeat."""
        self.init_display()
        self.start_refresh_server()

        while self.running:
            self._refresh_event.clear()
            img = self.fetch_preview()
            if img:
                self.display_image(img)
            else:
                logger.warning("No image received - retrying in %ds", config.REFRESH_INTERVAL)

            # Wait for refresh interval or manual trigger
            self._refresh_event.wait(timeout=config.REFRESH_INTERVAL)


def main() -> None:
    """Entry point with signal handling."""
    client = EInkClient()

    def handle_signal(signum: int, frame) -> None:
        logger.info("Received signal %d - shutting down", signum)
        client.shutdown()

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    logger.info("E-Ink Picture Client starting")
    logger.info("Server: %s", config.SERVER_URL)
    logger.info("Refresh interval: %ds", config.REFRESH_INTERVAL)
    logger.info("Deployment mode: %s", config.DEPLOYMENT_MODE)

    try:
        client.run()
    except Exception as e:
        logger.error("Fatal error: %s", e)
        client.shutdown()
        sys.exit(1)

    logger.info("Client stopped")


if __name__ == "__main__":
    main()
