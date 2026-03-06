#!/usr/bin/env python3
"""Tests for E-Ink Picture Client with mock display."""

import json
import time
import unittest
from io import BytesIO
from unittest.mock import MagicMock, patch, PropertyMock

from PIL import Image


class MockEPD:
    """Mock Waveshare EPD driver for testing without hardware."""

    def __init__(self):
        self.width = 800
        self.height = 480
        self.initialized = False
        self.sleeping = False
        self.displayed_buffer = None

    def init(self):
        self.initialized = True
        self.sleeping = False

    def display(self, buffer):
        self.displayed_buffer = buffer

    def getbuffer(self, image):
        return list(image.tobytes())

    def sleep(self):
        self.sleeping = True
        self.initialized = False


def make_test_png(width=800, height=480, color=(255, 255, 255)):
    """Create a test PNG image in memory."""
    img = Image.new("RGB", (width, height), color)
    buf = BytesIO()
    img.save(buf, format="PNG")
    buf.seek(0)
    return buf.getvalue()


class TestFetchPreview(unittest.TestCase):
    """Test preview fetching from server."""

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_success(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        png_data = make_test_png()
        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.status_code = 200
        mock_resp.content = png_data
        mock_resp.raise_for_status = MagicMock()
        mock_requests.get.return_value = mock_resp

        import client
        img = client.fetch_preview()

        self.assertIsNotNone(img)
        self.assertEqual(img.size, (800, 480))
        mock_requests.get.assert_called_once()

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_server_down(self, mock_config, mock_requests):
        import requests as real_requests
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_requests.ConnectionError = real_requests.ConnectionError
        mock_requests.get.side_effect = real_requests.ConnectionError("Connection refused")

        import client
        img = client.fetch_preview()

        self.assertIsNone(img)

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_server_error(self, mock_config, mock_requests):
        import requests as real_requests
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_resp = MagicMock()
        mock_resp.raise_for_status.side_effect = real_requests.HTTPError("500 Server Error")
        mock_requests.get.return_value = mock_resp
        mock_requests.ConnectionError = real_requests.ConnectionError

        import client
        img = client.fetch_preview()

        self.assertIsNone(img)


class TestDisplayImage(unittest.TestCase):
    """Test image display on mock EPD hardware."""

    def test_display_bw_image(self):
        """B/W display: image should be converted to mode '1'."""
        import client
        mock_epd = MockEPD()
        client.epd = mock_epd

        img = Image.new("RGB", (800, 480), (128, 128, 128))
        display_config = {"colors": ["#000000", "#FFFFFF"]}

        result = client.display_image(img, display_config)

        self.assertTrue(result)
        self.assertTrue(mock_epd.sleeping)
        self.assertIsNotNone(mock_epd.displayed_buffer)

    def test_display_color_image(self):
        """6-color display: image stays RGB."""
        import client
        mock_epd = MockEPD()
        client.epd = mock_epd

        img = Image.new("RGB", (800, 480), (255, 0, 0))
        display_config = {
            "colors": ["#000000", "#FFFFFF", "#FF0000", "#00FF00", "#0000FF", "#FFFF00"]
        }

        result = client.display_image(img, display_config)

        self.assertTrue(result)
        self.assertTrue(mock_epd.sleeping)

    def test_display_resize_if_needed(self):
        """Image should be resized to match display dimensions."""
        import client
        mock_epd = MockEPD()
        client.epd = mock_epd

        img = Image.new("RGB", (1600, 960), (0, 0, 0))
        display_config = {"colors": ["#000000", "#FFFFFF"]}

        result = client.display_image(img, display_config)

        self.assertTrue(result)

    def test_display_no_hardware(self):
        """Without hardware, image should be saved to file."""
        import client
        client.epd = None

        img = Image.new("RGB", (800, 480), (255, 255, 255))
        display_config = {}

        with patch.object(img, "save") as mock_save:
            result = client.display_image(img, display_config)

        self.assertTrue(result)

    def test_display_error_recovery(self):
        """Display error should return False and put display to sleep."""
        import client
        mock_epd = MockEPD()
        mock_epd.display = MagicMock(side_effect=Exception("SPI error"))
        client.epd = mock_epd

        img = Image.new("RGB", (800, 480), (0, 0, 0))
        display_config = {"colors": ["#000000", "#FFFFFF"]}

        result = client.display_image(img, display_config)

        self.assertFalse(result)


class TestRefreshStatus(unittest.TestCase):
    """Test server refresh polling."""

    @patch("client.requests")
    @patch("client.config")
    def test_should_refresh_true(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.json.return_value = {"should_refresh": True}
        mock_requests.get.return_value = mock_resp

        import client
        result = client.check_should_refresh()

        self.assertTrue(result)

    @patch("client.requests")
    @patch("client.config")
    def test_should_refresh_false(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.json.return_value = {"should_refresh": False}
        mock_requests.get.return_value = mock_resp

        import client
        result = client.check_should_refresh()

        self.assertFalse(result)

    @patch("client.requests")
    @patch("client.config")
    def test_should_refresh_server_error(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_requests.get.side_effect = Exception("timeout")

        import client
        result = client.check_should_refresh()

        self.assertFalse(result)


class TestHeartbeat(unittest.TestCase):
    """Test client heartbeat to server."""

    @patch("client.requests")
    @patch("client.config")
    def test_send_heartbeat(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"

        import client
        client.send_heartbeat()

        mock_requests.post.assert_called_once()
        call_args = mock_requests.post.call_args
        self.assertIn("/api/client_heartbeat", call_args[0][0])
        body = call_args[1]["json"]
        self.assertEqual(body["status"], "refreshed")
        self.assertIn("timestamp", body)

    @patch("client.requests")
    @patch("client.config")
    def test_send_heartbeat_server_down(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_requests.post.side_effect = Exception("Connection refused")

        import client
        # Should not raise
        client.send_heartbeat()


class TestFetchDisplayConfig(unittest.TestCase):
    """Test display config fetching."""

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_config_success(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.DISPLAY_DRIVER = "epd7in3e"

        import client
        client.driver_name = "epd7in3e"

        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.json.return_value = {
            "display": {
                "driver": "epd7in3e",
                "width": 800,
                "height": 480,
                "colors": ["#000000", "#FFFFFF", "#FF0000", "#00FF00", "#0000FF", "#FFFF00"],
            }
        }
        mock_requests.get.return_value = mock_resp

        result = client.fetch_display_config()

        self.assertEqual(result["driver"], "epd7in3e")
        self.assertEqual(result["width"], 800)

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_config_server_down(self, mock_config, mock_requests):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_requests.get.side_effect = Exception("Connection refused")

        import client
        result = client.fetch_display_config()

        self.assertEqual(result, {})


class TestMainLoop(unittest.TestCase):
    """Test main loop behavior."""

    @patch("client.send_heartbeat")
    @patch("client.display_image", return_value=True)
    @patch("client.fetch_preview")
    @patch("client.fetch_display_config", return_value={})
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_initial_update(self, mock_time, mock_signal, mock_config,
                            mock_load, mock_fetch_config, mock_fetch_preview,
                            mock_display, mock_heartbeat):
        """Main loop performs initial display update on startup."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 30

        test_img = Image.new("RGB", (800, 480), (255, 255, 255))
        mock_fetch_preview.return_value = test_img

        # Make the loop exit after initial update
        call_count = [0]
        def fake_sleep(seconds):
            call_count[0] += 1
            if call_count[0] >= 2:
                raise KeyboardInterrupt()
        mock_time.sleep.side_effect = fake_sleep
        mock_time.strftime = time.strftime
        mock_time.gmtime = time.gmtime

        import client
        try:
            client.main()
        except KeyboardInterrupt:
            pass

        mock_load.assert_called_once_with("epd7in3e")
        mock_fetch_preview.assert_called()
        mock_display.assert_called()
        mock_heartbeat.assert_called()

    @patch("client.send_heartbeat")
    @patch("client.display_image", return_value=True)
    @patch("client.fetch_preview", return_value=None)
    @patch("client.fetch_display_config", return_value={})
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_no_image_on_startup(self, mock_time, mock_signal, mock_config,
                                  mock_load, mock_fetch_config, mock_fetch_preview,
                                  mock_display, mock_heartbeat):
        """Main loop handles missing image gracefully."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 30

        call_count = [0]
        def fake_sleep(seconds):
            call_count[0] += 1
            if call_count[0] >= 2:
                raise KeyboardInterrupt()
        mock_time.sleep.side_effect = fake_sleep

        import client
        try:
            client.main()
        except KeyboardInterrupt:
            pass

        mock_display.assert_not_called()
        mock_heartbeat.assert_not_called()


if __name__ == "__main__":
    unittest.main()
