#!/usr/bin/env python3
"""E-Ink Picture Client — fetches rendered preview from server and displays on E-Ink."""

import logging
import signal
import sys
import time
from io import BytesIO
from typing import Optional

import requests
from PIL import Image

import config

logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL),
    format="%(asctime)s [%(levelname)s] %(message)s"
)
logger = logging.getLogger("eink-client")

# Display driver (loaded lazily)
epd = None
driver_name = config.DISPLAY_DRIVER


def load_display_driver(name: str):
    """Dynamically load the correct Waveshare EPD driver."""
    global epd, driver_name
    driver_name = name
    logger.info("Loading display driver: %s", name)
    try:
        if name == "epd7in3e":
            from waveshare_epd import epd7in3e
            epd = epd7in3e.EPD()
        elif name == "epd7in5_V2":
            from waveshare_epd import epd7in5_V2
            epd = epd7in5_V2.EPD()
        else:
            logger.error("Unknown display driver: %s", name)
            return
        logger.info("Display driver loaded: %s", name)
    except ImportError:
        logger.warning("Waveshare EPD library not found - running in preview-only mode")
        epd = None


def fetch_display_config() -> dict:
    """Fetch display config from server settings."""
    try:
        resp = requests.get(f"{config.SERVER_URL}/settings", timeout=5)
        if resp.ok:
            settings = resp.json()
            display = settings.get("display", {})
            driver = display.get("driver", config.DISPLAY_DRIVER)
            if driver != driver_name:
                load_display_driver(driver)
            return display
    except Exception as e:
        logger.warning("Could not fetch display settings: %s", e)
    return {}


def fetch_preview() -> Optional[Image.Image]:
    """Fetch rendered preview PNG from server."""
    try:
        resp = requests.get(f"{config.SERVER_URL}/preview", timeout=30)
        resp.raise_for_status()
        img = Image.open(BytesIO(resp.content))
        logger.info("Preview fetched: %dx%d, mode=%s", img.size[0], img.size[1], img.mode)
        return img
    except requests.ConnectionError:
        logger.warning("Server not reachable: %s", config.SERVER_URL)
    except Exception as e:
        logger.error("Failed to fetch preview: %s", e)
    return None


def display_image(img: Image.Image, display_config: dict) -> bool:
    """Send image to E-Ink display via SPI."""
    if not epd:
        img.save("preview_output.png")
        logger.info("No display hardware - preview saved to preview_output.png")
        return True

    try:
        logger.info("Initializing display...")
        epd.init()

        display_width = epd.width
        display_height = epd.height
        if img.size != (display_width, display_height):
            img = img.resize((display_width, display_height), Image.Resampling.LANCZOS)
            logger.info("Resized to %dx%d", display_width, display_height)

        colors = display_config.get("colors", ["#000000", "#FFFFFF"])
        if len(colors) > 2:
            # 6-color display: convert to RGB, driver handles palette internally
            if img.mode != "RGB":
                img = img.convert("RGB")
            logger.info("Sending to %d-color display...", len(colors))
        else:
            # B/W display
            if img.mode != "1":
                img = img.convert("1")
            logger.info("Sending to B/W display...")

        epd.display(epd.getbuffer(img))

        logger.info("Display entering sleep mode...")
        epd.sleep()
        logger.info("Display updated successfully")
        return True
    except Exception as e:
        logger.error("Display error: %s", e)
        try:
            epd.sleep()
        except Exception:
            pass
        return False


def check_should_refresh() -> bool:
    """Ask server if display should refresh."""
    try:
        resp = requests.get(f"{config.SERVER_URL}/api/refresh_status", timeout=5)
        if resp.ok:
            data = resp.json()
            return data.get("should_refresh", False)
    except Exception:
        pass
    return False


def send_heartbeat() -> None:
    """Tell server that display was updated."""
    try:
        requests.post(
            f"{config.SERVER_URL}/api/client_heartbeat",
            json={
                "status": "refreshed",
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            },
            timeout=5,
        )
    except Exception:
        pass


def cleanup() -> None:
    """Clean shutdown — put display to sleep and release GPIO."""
    if epd:
        try:
            epd.sleep()
        except Exception:
            pass
        try:
            if driver_name == "epd7in3e":
                from waveshare_epd import epd7in3e
                epd7in3e.epdconfig.module_exit()
            elif driver_name == "epd7in5_V2":
                from waveshare_epd import epd7in5_V2
                epd7in5_V2.epdconfig.module_exit()
        except Exception:
            pass


def main() -> None:
    """Main loop: poll server for refresh status, fetch preview, display, repeat."""
    logger.info("E-Ink Client starting - Server: %s, Driver: %s", config.SERVER_URL, config.DISPLAY_DRIVER)

    running = True

    def shutdown(signum, frame):
        nonlocal running
        logger.info("Shutting down...")
        running = False

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    # Initial setup: load driver and fetch config
    load_display_driver(config.DISPLAY_DRIVER)
    display_config = fetch_display_config()
    if not display_config:
        display_config = {}

    # Initial display update
    logger.info("Performing initial display update...")
    img = fetch_preview()
    if img:
        if display_image(img, display_config):
            send_heartbeat()
    else:
        logger.warning("No image on startup - will retry on next poll")

    # Main poll loop
    poll_interval = config.POLL_INTERVAL
    logger.info("Entering poll loop (every %ds)", poll_interval)

    while running:
        for _ in range(poll_interval):
            if not running:
                break
            time.sleep(1)

        if not running:
            break

        if check_should_refresh():
            logger.info("Server says: refresh needed")
            display_config = fetch_display_config()
            img = fetch_preview()
            if img:
                if display_image(img, display_config):
                    send_heartbeat()
            else:
                logger.warning("Failed to fetch preview for refresh")

    cleanup()
    logger.info("Client stopped")


if __name__ == "__main__":
    main()
