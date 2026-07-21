#!/usr/bin/env python3
"""Tests for E-Ink Picture Client with mock display."""

import hashlib
import importlib
import json
import logging
import os
import sys
import tempfile
import time
import unittest
from io import BytesIO
from unittest.mock import MagicMock, patch, PropertyMock

from PIL import Image

DEFAULT_LAST_SENT_PATH = "/tmp/eink_last_sent.png"

COLOR_DISPLAY_CONFIG = {
    "colors": ["#000000", "#FFFFFF", "#FF0000", "#00FF00", "#0000FF", "#FFFF00"]
}
BW_DISPLAY_CONFIG = {"colors": ["#000000", "#FFFFFF"]}

# The 6-color panel palette as RGB tuples (matches COLOR_DISPLAY_CONFIG).
PANEL_PALETTE_RGB = [
    (0, 0, 0),
    (255, 255, 255),
    (255, 0, 0),
    (0, 255, 0),
    (0, 0, 255),
    (255, 255, 0),
]


def _snapshot_default_artifact_paths():
    """Existence/mtime of the real default artifact path and its temp sibling."""
    state = {}
    for path in (DEFAULT_LAST_SENT_PATH, DEFAULT_LAST_SENT_PATH + ".tmp"):
        state[path] = os.path.getmtime(path) if os.path.exists(path) else None
    return state


_default_artifact_before = None


def setUpModule():
    global _default_artifact_before
    _default_artifact_before = _snapshot_default_artifact_paths()


def tearDownModule():
    """AC7 guard: no test may create or modify the real default /tmp artifact."""
    after = _snapshot_default_artifact_paths()
    if after != _default_artifact_before:
        raise AssertionError(
            "AC7 violated: test suite touched the default artifact path "
            f"(before={_default_artifact_before}, after={after})"
        )


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


class RecordingEPD(MockEPD):
    """MockEPD that records the exact PIL image passed to getbuffer().

    Optionally also records whether the artifact file already existed at the
    moment display() was called (write point must be before the driver call).
    """

    def __init__(self, artifact_path=None):
        super().__init__()
        self.artifact_path = artifact_path
        self.getbuffer_image = None
        self.artifact_existed_at_display = None

    def getbuffer(self, image):
        self.getbuffer_image = image.copy()
        return super().getbuffer(image)

    def display(self, buffer):
        if self.artifact_path is not None:
            self.artifact_existed_at_display = os.path.exists(self.artifact_path)
        super().display(buffer)


class CountingEPD(RecordingEPD):
    """RecordingEPD that counts init/display/sleep calls (E5.2 skip-path proof)."""

    def __init__(self, artifact_path=None):
        super().__init__(artifact_path)
        self.init_calls = 0
        self.display_calls = 0
        self.sleep_calls = 0

    def init(self):
        self.init_calls += 1
        super().init()

    def display(self, buffer):
        self.display_calls += 1
        super().display(buffer)

    def sleep(self):
        self.sleep_calls += 1
        super().sleep()


class FailingEPD(CountingEPD):
    """CountingEPD that fails in one configurable driver call (E5.4).

    fail_on: "init" (raises), "init_minus_one" (returns -1 like the real
    drivers when module_init fails), "getbuffer" or "display" (raise).
    init/display count the ATTEMPT before failing; getbuffer failures are
    not counted (no counter exists for it).
    """

    def __init__(self, fail_on="init", exc=None, artifact_path=None):
        super().__init__(artifact_path)
        self.fail_on = fail_on
        self.exc = exc if exc is not None else OSError("SPI transfer failed")

    def init(self):
        if self.fail_on == "init":
            self.init_calls += 1
            raise self.exc
        if self.fail_on == "init_minus_one":
            self.init_calls += 1
            return -1
        return super().init()

    def getbuffer(self, image):
        if self.fail_on == "getbuffer":
            raise self.exc
        return super().getbuffer(image)

    def display(self, buffer):
        if self.fail_on == "display":
            self.display_calls += 1
            raise self.exc
        super().display(buffer)


def make_test_png(width=800, height=480, color=(255, 255, 255)):
    """Create a test PNG image in memory."""
    img = Image.new("RGB", (width, height), color)
    buf = BytesIO()
    img.save(buf, format="PNG")
    buf.seek(0)
    return buf.getvalue()


def make_gradient_image(width=800, height=480):
    """Create a non-uniform RGB test image so pixel-identity checks are meaningful."""
    base = Image.linear_gradient("L").resize((width, height))
    return Image.merge(
        "RGB",
        (
            base,
            base.transpose(Image.Transpose.ROTATE_180),
            base.transpose(Image.Transpose.FLIP_LEFT_RIGHT),
        ),
    )


def make_paletted_panel_image(width=400, height=240):
    """Mode-'P' image with the 6-color panel palette and a high-frequency pattern.

    Adjacent pixels always differ (diagonal stripes over all 6 palette
    indices), so any interpolating resample would create mixed colors.
    """
    img = Image.new("P", (width, height))
    img.putpalette([channel for rgb in PANEL_PALETTE_RGB for channel in rgb])
    img.putdata([(x + y) % 6 for y in range(height) for x in range(width)])
    return img


def make_rgb_panel_image(width=400, height=240):
    """RGB image containing only the 6 panel palette colors, high-frequency pattern."""
    img = Image.new("RGB", (width, height))
    img.putdata(
        [PANEL_PALETTE_RGB[(x + y) % 6] for y in range(height) for x in range(width)]
    )
    return img


def make_bw_rgb_image(width=400, height=240):
    """RGB image with pure 0/255 pixels only (high-frequency checkerboard)."""
    img = Image.new("RGB", (width, height))
    img.putdata(
        [
            (0, 0, 0) if (x + y) % 2 == 0 else (255, 255, 255)
            for y in range(height)
            for x in range(width)
        ]
    )
    return img


class ArtifactSandboxMixin:
    """Redirect config.LAST_SENT_PATH into a per-test temp directory (AC7).

    Every test that can reach display_image() must use this mixin so no test
    ever writes to the real default /tmp path. Also restores client.epd and
    the E5.4 recovery state (display_image failures mutate module globals).
    """

    def setUp(self):
        super().setUp()
        import client
        import config
        self.client = client
        tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(tmpdir.cleanup)
        self.artifact_dir = tmpdir.name
        self.artifact_path = os.path.join(self.artifact_dir, "eink_last_sent.png")
        patcher = patch.object(config, "LAST_SENT_PATH", self.artifact_path)
        patcher.start()
        self.addCleanup(patcher.stop)
        self.addCleanup(setattr, client, "epd", client.epd)
        for attr in ("_preview_only", "_hw_recovery_pending",
                     "_consecutive_hw_failures", "_initial_display_done"):
            self.addCleanup(setattr, client, attr, getattr(client, attr))


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

    def _mock_ok_png(self, mock_requests):
        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.status_code = 200
        mock_resp.content = make_test_png()
        mock_resp.raise_for_status = MagicMock()
        mock_requests.get.return_value = mock_resp

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_original_appends_raw(self, mock_config, mock_requests):
        """F10 AC5: mode=original requests /preview?raw=true."""
        mock_config.SERVER_URL = "http://localhost:5000"
        self._mock_ok_png(mock_requests)

        import client
        client.fetch_preview("original")

        url = mock_requests.get.call_args.args[0]
        self.assertEqual(url, "http://localhost:5000/preview?raw=true")

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_dithered_no_raw(self, mock_config, mock_requests):
        """F10 AC5: mode=dithered requests /preview unchanged."""
        mock_config.SERVER_URL = "http://localhost:5000"
        self._mock_ok_png(mock_requests)

        import client
        client.fetch_preview("dithered")

        url = mock_requests.get.call_args.args[0]
        self.assertEqual(url, "http://localhost:5000/preview")
        self.assertNotIn("raw", url)

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_default_no_raw(self, mock_config, mock_requests):
        """F10 AC5: default (no arg) stays dithered — /preview, no raw."""
        mock_config.SERVER_URL = "http://localhost:5000"
        self._mock_ok_png(mock_requests)

        import client
        client.fetch_preview()

        url = mock_requests.get.call_args.args[0]
        self.assertEqual(url, "http://localhost:5000/preview")

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_preview_unknown_mode_no_raw(self, mock_config, mock_requests):
        """F10 robustness: an unexpected mode value behaves as dithered."""
        mock_config.SERVER_URL = "http://localhost:5000"
        self._mock_ok_png(mock_requests)

        import client
        client.fetch_preview("bogus")

        url = mock_requests.get.call_args.args[0]
        self.assertEqual(url, "http://localhost:5000/preview")
        self.assertNotIn("raw", url)


class TestDisplayImage(ArtifactSandboxMixin, unittest.TestCase):
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


class TestLastSentPathConfig(unittest.TestCase):
    """AC1: config.LAST_SENT_PATH default value and env override."""

    def tearDown(self):
        # Restore module state from the real environment after reload tests.
        import config
        importlib.reload(config)

    def test_last_sent_path_default(self):
        """Without EINK_LAST_SENT_PATH the default is /tmp/eink_last_sent.png."""
        import config
        with patch.dict(os.environ):
            os.environ.pop("EINK_LAST_SENT_PATH", None)
            importlib.reload(config)
            self.assertEqual(config.LAST_SENT_PATH, DEFAULT_LAST_SENT_PATH)

    def test_last_sent_path_env_override(self):
        """EINK_LAST_SENT_PATH overrides the default after module reload."""
        import config
        override = "/custom/debug/last_sent.png"
        with patch.dict(os.environ, {"EINK_LAST_SENT_PATH": override}):
            importlib.reload(config)
            self.assertEqual(config.LAST_SENT_PATH, override)


class TestLastSentArtifact(ArtifactSandboxMixin, unittest.TestCase):
    """AC2-AC6: last-sent debug artifact written by display_image()."""

    def test_last_sent_artifact_color(self):
        """AC2/AC3: artifact is pixel-identical to the RGB image given to the driver.

        Oversized non-uniform input proves the write point is after resize;
        RecordingEPD proves it is before epd.display().
        """
        mock_epd = RecordingEPD(artifact_path=self.artifact_path)
        self.client.epd = mock_epd

        img = make_gradient_image(1600, 960)
        result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertTrue(os.path.exists(self.artifact_path))
        with Image.open(self.artifact_path) as artifact:
            artifact.load()
            self.assertEqual(artifact.size, (800, 480))
            self.assertEqual(artifact.mode, "RGB")
            self.assertEqual(artifact.tobytes(), mock_epd.getbuffer_image.tobytes())
        # Write point: artifact was already on disk when epd.display() ran.
        self.assertTrue(mock_epd.artifact_existed_at_display)
        # No leftover temp files next to the artifact.
        self.assertEqual(os.listdir(self.artifact_dir), ["eink_last_sent.png"])

    def test_last_sent_artifact_bw(self):
        """AC3: B/W path writes a mode-'1' artifact, pixel-identical to driver input."""
        mock_epd = RecordingEPD(artifact_path=self.artifact_path)
        self.client.epd = mock_epd

        img = make_gradient_image()
        result = self.client.display_image(img, BW_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertEqual(mock_epd.getbuffer_image.mode, "1")
        self.assertTrue(os.path.exists(self.artifact_path))
        with Image.open(self.artifact_path) as artifact:
            artifact.load()
            self.assertEqual(artifact.size, (800, 480))
            self.assertEqual(artifact.mode, "1")
            self.assertEqual(artifact.tobytes(), mock_epd.getbuffer_image.tobytes())
            # 1-bit artifact: only pure black/white pixels survive the threshold.
            self.assertEqual(set(artifact.convert("L").getdata()) - {0, 255}, set())
        self.assertTrue(mock_epd.artifact_existed_at_display)

    def test_last_sent_artifact_atomic(self):
        """AC4: temp file in the target directory + os.replace, never a direct save."""
        mock_epd = RecordingEPD(artifact_path=self.artifact_path)
        self.client.epd = mock_epd
        img = make_gradient_image()

        with patch("client.os.replace") as mock_replace:
            result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        mock_replace.assert_called_once()
        src, dst = mock_replace.call_args[0]
        self.assertEqual(dst, self.artifact_path)
        self.assertNotEqual(src, dst)
        # Temp file must live in the same directory (os.replace atomicity).
        self.assertEqual(os.path.dirname(src), os.path.dirname(dst))
        # os.replace was a no-op, so the temp file is still there: it must be
        # a complete PNG identical to the image passed to the driver.
        self.assertTrue(os.path.exists(src))
        with Image.open(src) as tmp_png:
            tmp_png.load()
            self.assertEqual(tmp_png.tobytes(), mock_epd.getbuffer_image.tobytes())
        # The target was never written directly (would exist otherwise).
        self.assertFalse(os.path.exists(self.artifact_path))

    def test_last_sent_artifact_write_failure_does_not_break_refresh(self):
        """AC5: save into a missing directory logs a warning, refresh continues."""
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd
        img = make_gradient_image()
        missing = os.path.join(self.artifact_dir, "does-not-exist", "eink_last_sent.png")

        import config
        with patch.object(config, "LAST_SENT_PATH", missing):
            with self.assertLogs("eink-client", level="WARNING") as logs:
                result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.displayed_buffer)
        self.assertTrue(mock_epd.sleeping)
        self.assertTrue(
            any(
                record.levelno == logging.WARNING and missing in record.getMessage()
                for record in logs.records
            ),
            f"expected WARNING mentioning {missing}, got: {logs.output}",
        )
        self.assertFalse(os.path.exists(missing))

    def test_last_sent_artifact_replace_failure_does_not_break_refresh(self):
        """AC5: os.replace failure logs a warning, epd.display() still runs."""
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd
        img = make_gradient_image()

        with patch("client.os.replace", side_effect=OSError("read-only filesystem")):
            with self.assertLogs("eink-client", level="WARNING") as logs:
                result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.displayed_buffer)
        self.assertTrue(
            any(
                record.levelno == logging.WARNING
                and self.artifact_path in record.getMessage()
                for record in logs.records
            ),
            f"expected WARNING mentioning {self.artifact_path}, got: {logs.output}",
        )
        self.assertFalse(os.path.exists(self.artifact_path))

    def test_no_artifact_without_hardware(self):
        """AC6: with epd is None only preview_output.png is saved, no artifact."""
        self.client.epd = None
        img = make_gradient_image()

        with patch("client.os.replace") as mock_replace, \
                patch.object(img, "save") as mock_save:
            result = self.client.display_image(img, {})

        self.assertTrue(result)
        mock_save.assert_called_once_with("preview_output.png")
        mock_replace.assert_not_called()
        self.assertFalse(os.path.exists(self.artifact_path))
        self.assertEqual(os.listdir(self.artifact_dir), [])


class TestResizeGuard(ArtifactSandboxMixin, unittest.TestCase):
    """E1.4 AC1-AC6: size mismatch must never destroy server-side dithering."""

    def _unique_rgb_colors(self, img):
        """Set of unique RGB tuples in an image (any mode)."""
        colors = img.convert("RGB").getcolors(1_000_000)
        self.assertIsNotNone(colors, "more than 1,000,000 unique colors")
        return {rgb for _, rgb in colors}

    def _assert_single_mismatch_warning(self, logs):
        """AC4: exactly one WARNING containing both actual and target size."""
        warnings = [r for r in logs.records if r.levelno == logging.WARNING]
        self.assertEqual(
            len(warnings), 1,
            f"expected exactly one WARNING, got: {logs.output}",
        )
        message = warnings[0].getMessage()
        self.assertIn("400x240", message)
        self.assertIn("800x480", message)

    def test_resize_mismatch_paletted_input_preserves_palette(self):
        """AC1/AC4: mode-'P' input in foreign size keeps only palette colors."""
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd

        img = make_paletted_panel_image(400, 240)
        with self.assertLogs("eink-client", level="WARNING") as logs:
            result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.displayed_buffer)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertEqual(mock_epd.getbuffer_image.size, (800, 480))
        unique = self._unique_rgb_colors(mock_epd.getbuffer_image)
        self.assertLessEqual(
            unique, set(PANEL_PALETTE_RGB),
            f"non-palette colors survived the resize: {sorted(unique - set(PANEL_PALETTE_RGB))[:10]}",
        )
        self._assert_single_mismatch_warning(logs)

    def test_resize_mismatch_rgb_input_preserves_palette(self):
        """AC2/AC4: RGB input in foreign size keeps only palette colors.

        Regression proof against LANCZOS: an interpolating resample of this
        high-frequency image produces hundreds of mixed colors (spec context
        point 4) - this test must be red against the old code.
        """
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd

        img = make_rgb_panel_image(400, 240)
        with self.assertLogs("eink-client", level="WARNING") as logs:
            result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertEqual(mock_epd.getbuffer_image.size, (800, 480))
        unique = self._unique_rgb_colors(mock_epd.getbuffer_image)
        self.assertLessEqual(
            unique, set(PANEL_PALETTE_RGB),
            f"non-palette colors survived the resize: {sorted(unique - set(PANEL_PALETTE_RGB))[:10]}",
        )
        self._assert_single_mismatch_warning(logs)

    def test_resize_mismatch_bw_threshold_after_resize(self):
        """AC3/AC4: B/W path resizes with NEAREST, threshold acts on the scaled image."""
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd

        img = make_bw_rgb_image(400, 240)
        with self.assertLogs("eink-client", level="WARNING") as logs:
            result = self.client.display_image(img, BW_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertEqual(mock_epd.getbuffer_image.mode, "1")
        self.assertEqual(mock_epd.getbuffer_image.size, (800, 480))
        gray_values = set(mock_epd.getbuffer_image.convert("L").tobytes())
        self.assertEqual(gray_values - {0, 255}, set())
        self._assert_single_mismatch_warning(logs)

    def test_no_warning_on_exact_size(self):
        """AC5: exact-size input triggers no resize, no WARNING, byte-identical output."""
        mock_epd = RecordingEPD()
        self.client.epd = mock_epd

        img = make_gradient_image(800, 480)
        expected_bytes = img.tobytes()
        with patch.object(img, "resize", wraps=img.resize) as resize_spy:
            with self.assertNoLogs("eink-client", level="WARNING"):
                result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        resize_spy.assert_not_called()
        self.assertIsNotNone(mock_epd.getbuffer_image)
        # RGB input on the 6-color path passes through unchanged.
        self.assertEqual(mock_epd.getbuffer_image.tobytes(), expected_bytes)

    def test_resize_artifact_matches_driver_image(self):
        """AC6: in the mismatch case the E1.2 artifact equals the driver image."""
        mock_epd = RecordingEPD(artifact_path=self.artifact_path)
        self.client.epd = mock_epd

        img = make_rgb_panel_image(400, 240)
        result = self.client.display_image(img, COLOR_DISPLAY_CONFIG)

        self.assertTrue(result)
        self.assertIsNotNone(mock_epd.getbuffer_image)
        self.assertTrue(os.path.exists(self.artifact_path))
        with Image.open(self.artifact_path) as artifact:
            artifact.load()
            self.assertEqual(artifact.size, (800, 480))
            self.assertEqual(artifact.mode, mock_epd.getbuffer_image.mode)
            self.assertEqual(artifact.tobytes(), mock_epd.getbuffer_image.tobytes())
        self.assertTrue(mock_epd.artifact_existed_at_display)


class FakeSkipServer:
    """Dispatching fake for client._server_get/_server_post (E5.2 tests).

    Serves refresh_status (should_refresh/reason), display settings and the
    current PNG bytes; records every heartbeat payload.
    """

    def __init__(self, png_bytes, reason="interval", should_refresh=True):
        self.png_bytes = png_bytes
        self.reason = reason  # None = field absent (old server, version skew)
        self.should_refresh = should_refresh
        self.heartbeats = []

    def get(self, path, timeout=None):
        resp = MagicMock()
        resp.status_code = 200
        resp.ok = True
        resp.raise_for_status = MagicMock()
        if path == "/api/refresh_status":
            body = {"should_refresh": self.should_refresh, "refresh_interval": 3600}
            if self.reason is not None:
                body["reason"] = self.reason
            resp.json.return_value = body
        elif path == "/settings":
            resp.json.return_value = {
                "display": {
                    "driver": "epd7in3e",
                    "width": 800,
                    "height": 480,
                    "colors": COLOR_DISPLAY_CONFIG["colors"],
                }
            }
        elif path == "/preview":
            resp.content = self.png_bytes
        else:
            raise AssertionError(f"unexpected GET {path}")
        return resp

    def post(self, path, payload, timeout=None):
        if path != "/api/client_heartbeat":
            raise AssertionError(f"unexpected POST {path}")
        self.heartbeats.append(payload)
        resp = MagicMock()
        resp.status_code = 200
        resp.ok = True
        return resp


class ContentSkipSandbox(ArtifactSandboxMixin):
    """E5.2/E5.4 shared setup: CountingEPD, fake server, fresh in-memory state.

    load_display_driver is replaced by a fake that (re-)installs whatever
    self.epd currently references - the E5.4 recovery re-load path thus gets
    a controllable "fresh EPD() from the cached module" without touching the
    real waveshare imports. Tests swap self.epd to steer the next re-load.
    """

    def setUp(self):
        super().setUp()
        import config
        self.config = config
        client = self.client
        for attr in ("_last_fetch_hash", "_last_displayed_hash",
                     "_last_panel_write_monotonic", "driver_name"):
            self.addCleanup(setattr, client, attr, getattr(client, attr))
        client._last_fetch_hash = None
        client._last_displayed_hash = None
        client._last_panel_write_monotonic = None
        client.driver_name = "epd7in3e"
        # E5.4 state: post-startup semantics by default (initial write done),
        # no pending recovery, counter at zero.
        client._preview_only = False
        client._hw_recovery_pending = False
        client._consecutive_hw_failures = 0
        client._initial_display_done = True
        self.epd = CountingEPD(artifact_path=self.artifact_path)
        client.epd = self.epd
        self.server = FakeSkipServer(make_test_png(color=(10, 20, 30)))
        for name, handler in (("_server_get", self.server.get),
                              ("_server_post", self.server.post)):
            patcher = patch.object(client, name, side_effect=handler)
            patcher.start()
            self.addCleanup(patcher.stop)
        self.real_load_display_driver = client.load_display_driver

        def _fake_load(name):
            client.driver_name = name
            client.epd = self.epd
            client._preview_only = False
            client._hw_recovery_pending = False

        self.load_patcher = patch.object(
            client, "load_display_driver", side_effect=_fake_load
        )
        self.load_mock = self.load_patcher.start()
        self.addCleanup(self.load_patcher.stop)

    def heartbeat_statuses(self):
        return [hb["status"] for hb in self.server.heartbeats]

    def install_epd(self, epd):
        """Make epd the current driver object for client and future re-loads."""
        self.epd = epd
        self.client.epd = epd


class TestContentSkip(ContentSkipSandbox, unittest.TestCase):
    """E5.2 AC6-AC8: content skip, forced writes, hash-after-success."""

    def test_skip_second_cycle_identical_content(self):
        """AC6: identical bytes + reason interval => full skip on cycle 2."""
        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.init_calls, 1)
        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.epd.sleep_calls, 1)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])
        self.assertTrue(os.path.exists(self.artifact_path))
        artifact_mtime_ns = os.stat(self.artifact_path).st_mtime_ns
        with open(self.artifact_path, "rb") as fh:
            artifact_bytes = fh.read()

        with self.assertLogs("eink-client", level="INFO") as logs:
            self.client.process_refresh_cycle()

        # Panel stays in deep sleep: no init, no display, no sleep.
        self.assertEqual(self.epd.init_calls, 1)
        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.epd.sleep_calls, 1)
        # Heartbeat is still sent, with status "skipped".
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped"])
        self.assertIn("timestamp", self.server.heartbeats[1])
        # Last-sent artifact untouched (mtime and content).
        self.assertEqual(os.stat(self.artifact_path).st_mtime_ns, artifact_mtime_ns)
        with open(self.artifact_path, "rb") as fh:
            self.assertEqual(fh.read(), artifact_bytes)
        # Stable log line (HIL grep base).
        self.assertTrue(
            any(
                record.levelno == logging.INFO
                and "skipping panel refresh (content unchanged)" in record.getMessage()
                for record in logs.records
            ),
            f"expected skip INFO log, got: {logs.output}",
        )

    def test_no_refresh_when_server_says_false(self):
        """Sanity: should_refresh=false => no preview fetch, no write, no heartbeat."""
        self.server.should_refresh = False
        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 0)
        self.assertEqual(self.heartbeat_statuses(), [])
        get_paths = [c.args[0] for c in self.client._server_get.call_args_list]
        self.assertNotIn("/preview", get_paths)

    def test_changed_content_forces_write(self):
        """AC7a: different bytes => display runs, heartbeat "refreshed"."""
        self.client.process_refresh_cycle()
        self.server.png_bytes = make_test_png(color=(200, 0, 0))

        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])

    def test_skip_hash_updated_only_after_success(self):
        """AC7b/c: display failure keeps the old hash, sends no heartbeat,
        and the next cycle with the same bytes writes again (no skip on
        never-shown content)."""
        self.client.process_refresh_cycle()
        hash_a = hashlib.sha256(self.server.png_bytes).hexdigest()
        self.assertEqual(self.client._last_displayed_hash, hash_a)

        self.server.png_bytes = make_test_png(color=(0, 99, 0))
        hash_b = hashlib.sha256(self.server.png_bytes).hexdigest()
        with patch.object(self.epd, "display", side_effect=Exception("SPI error")):
            self.client.process_refresh_cycle()

        # Old hash unchanged, no heartbeat for the failed cycle (AC7c).
        self.assertEqual(self.client._last_displayed_hash, hash_a)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])

        # Same bytes again: must write, not skip.
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._last_displayed_hash, hash_b)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])

        # Now the content has really been shown once: next cycle skips.
        self.client.process_refresh_cycle()
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed", "skipped"])

    def test_manual_reason_forces_write(self):
        """AC8: reason "manual" => panel write despite identical bytes."""
        self.client.process_refresh_cycle()
        self.server.reason = "manual"

        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])

    def test_missing_reason_forces_write(self):
        """AC8: missing reason field (old server) => never skip (version skew)."""
        self.client.process_refresh_cycle()
        self.server.reason = None

        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])


class TestContentSkipGuardsAndSwitches(ContentSkipSandbox, unittest.TestCase):
    """E5.2 AC9/AC10: 24h guard, kill switch, restart state, no-hardware path."""

    def test_max_skip_hours_guard_forces_write(self):
        """AC9: last panel write older than MAX_SKIP_HOURS => write despite
        identical bytes; afterwards the guard clock is reset."""
        fake_now = [1000.0]
        with patch("client.time.monotonic", side_effect=lambda: fake_now[0]), \
                patch.object(self.config, "MAX_SKIP_HOURS", 24):
            self.client.process_refresh_cycle()
            fake_now[0] += 30.0
            self.client.process_refresh_cycle()  # skip
            fake_now[0] += 24 * 3600 + 1.0
            self.client.process_refresh_cycle()  # guard expired => write
            fake_now[0] += 30.0
            self.client.process_refresh_cycle()  # guard clock reset => skip again

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(
            self.heartbeat_statuses(),
            ["refreshed", "skipped", "refreshed", "skipped"],
        )

    def test_max_skip_hours_zero_disables_guard(self):
        """AC9: MAX_SKIP_HOURS=0 => guard off, skip stays skip."""
        fake_now = [1000.0]
        with patch("client.time.monotonic", side_effect=lambda: fake_now[0]), \
                patch.object(self.config, "MAX_SKIP_HOURS", 0):
            self.client.process_refresh_cycle()
            fake_now[0] += 1000 * 3600.0
            self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped"])

    def test_kill_switch_disables_skip(self):
        """AC10a: CONTENT_SKIP=False => identical bytes are always written."""
        with patch.object(self.config, "CONTENT_SKIP", False):
            self.client.process_refresh_cycle()
            self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])

    def test_restart_state_always_writes_first_frame(self):
        """AC10b: fresh process state => first frame is always written."""
        self.client.process_refresh_cycle()
        self.client.process_refresh_cycle()
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped"])

        # Simulate a process restart: in-memory state gone, nothing persisted.
        self.client._last_fetch_hash = None
        self.client._last_displayed_hash = None
        self.client._last_panel_write_monotonic = None

        self.client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped", "refreshed"])

    def test_no_hardware_never_skips(self):
        """AC10c: epd is None (preview-only) => skip logic inactive."""
        self.client.epd = None

        with patch.object(Image.Image, "save"):
            self.client.process_refresh_cycle()
            self.client.process_refresh_cycle()

        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])
        # The preview-only path never feeds the skip state.
        self.assertIsNone(self.client._last_displayed_hash)
        self.assertFalse(os.path.exists(self.artifact_path))


class TestDriverRecovery(ContentSkipSandbox, unittest.TestCase):
    """E5.4 AC1/AC2: driver exceptions => logger.exception + full driver reset."""

    def _run_failing_cycle(self, failing_epd):
        """One refresh cycle against a failing driver; returns captured logs."""
        self.install_epd(failing_epd)
        with patch.object(self.client, "_module_exit_best_effort") as module_exit, \
                self.assertLogs("eink-client", level="ERROR") as logs:
            self.client.process_refresh_cycle()
        self.module_exit_mock = module_exit
        return logs

    def _assert_recovery_state(self, logs, failing_epd):
        """Shared AC1/AC2 assertions after one failed cycle."""
        recovery_logs = [
            r for r in logs.records
            if "display recovery: driver reset after error" in r.getMessage()
        ]
        self.assertEqual(len(recovery_logs), 1, f"got: {logs.output}")
        self.assertIsNotNone(
            recovery_logs[0].exc_info, "logger.exception must attach a traceback"
        )
        self.module_exit_mock.assert_called_once()
        self.assertEqual(
            failing_epd.sleep_calls, 0, "no epd.sleep over a broken bus"
        )
        self.assertIsNone(self.client.epd)
        self.assertEqual(self.heartbeat_statuses(), [], "no heartbeat on failure")
        self.assertIsNone(
            self.client._last_displayed_hash, "no hash update on failure (E5.2)"
        )

    def test_init_failure_recovers_next_cycle(self):
        """AC1: OSError in init() => recovery; next cycle re-loads and writes."""
        failing = FailingEPD(fail_on="init")
        logs = self._run_failing_cycle(failing)
        self._assert_recovery_state(logs, failing)
        self.assertEqual(self.client._consecutive_hw_failures, 1)

        # Next cycle: the re-load delivers a fresh CountingEPD and writes.
        self.epd = CountingEPD(artifact_path=self.artifact_path)
        self.client.process_refresh_cycle()

        self.load_mock.assert_called_once_with("epd7in3e")
        self.assertEqual(self.epd.init_calls, 1)
        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.epd.sleep_calls, 1)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])
        self.assertEqual(self.client._consecutive_hw_failures, 0)

    def test_display_failure_recovers(self):
        """AC2: exception in display() - no sleep after the failed display,
        hash unchanged, artifact may exist (write point is before display)."""
        failing = FailingEPD(fail_on="display", artifact_path=self.artifact_path)
        logs = self._run_failing_cycle(failing)
        self._assert_recovery_state(logs, failing)
        self.assertTrue(os.path.exists(self.artifact_path))
        self.assertEqual(self.client._consecutive_hw_failures, 1)

        # Recovery works identically: next cycle re-loads and writes.
        self.epd = CountingEPD(artifact_path=self.artifact_path)
        self.client.process_refresh_cycle()
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])
        self.assertEqual(self.client._consecutive_hw_failures, 0)

    def test_getbuffer_failure_recovers(self):
        """AC2: exception in getbuffer() takes the identical recovery path."""
        failing = FailingEPD(fail_on="getbuffer")
        logs = self._run_failing_cycle(failing)
        self._assert_recovery_state(logs, failing)
        self.assertEqual(failing.display_calls, 0)
        self.assertEqual(self.client._consecutive_hw_failures, 1)

    def test_init_minus_one_treated_as_failure(self):
        """Decision 3: init() returning -1 goes through the same recovery path
        (MockEPD's None return and the real drivers' 0 must both pass)."""
        failing = FailingEPD(fail_on="init_minus_one")
        logs = self._run_failing_cycle(failing)
        self._assert_recovery_state(logs, failing)
        self.assertEqual(failing.display_calls, 0)
        self.assertEqual(self.client._consecutive_hw_failures, 1)


class TestHwEscalation(ContentSkipSandbox, unittest.TestCase):
    """E5.4 AC3/AC4: escalation counter, SystemExit(1), limit 0, preview-only."""

    def setUp(self):
        super().setUp()
        limit_patcher = patch.object(self.config, "HW_FAILURE_LIMIT", 3)
        limit_patcher.start()
        self.addCleanup(limit_patcher.stop)

    def test_escalates_after_three_consecutive_failures(self):
        """AC3: cycles 1+2 recover, cycle 3 raises SystemExit(1) + CRITICAL log."""
        self.install_epd(FailingEPD(fail_on="init"))

        self.client.process_refresh_cycle()
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 2)

        with self.assertLogs("eink-client", level="CRITICAL") as logs, \
                self.assertRaises(SystemExit) as cm:
            self.client.process_refresh_cycle()

        self.assertEqual(cm.exception.code, 1)
        self.assertTrue(
            any(
                r.levelno == logging.CRITICAL
                and "too many consecutive display failures" in r.getMessage()
                for r in logs.records
            ),
            f"expected CRITICAL escalation log, got: {logs.output}",
        )
        self.assertEqual(self.heartbeat_statuses(), [])

    def test_success_resets_counter(self):
        """AC3: fail-fail-success-fail-fail stays below the limit (reset on success)."""
        self.install_epd(FailingEPD(fail_on="init"))
        self.client.process_refresh_cycle()
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 2)

        # Cycle 3: driver swap => the re-load delivers a working EPD, write succeeds.
        self.epd = CountingEPD(artifact_path=self.artifact_path)
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 0)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])

        # Cycles 4+5: new bytes (no content skip) + failures again - the
        # counter restarts at 1, no SystemExit.
        self.server.png_bytes = make_test_png(color=(0, 0, 200))
        self.install_epd(FailingEPD(fail_on="init"))
        self.client.process_refresh_cycle()
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 2)

    def test_skip_cycles_do_not_change_counter(self):
        """AC3: content-skip cycles between failures leave the counter untouched."""
        # Cycle 1: successful write of bytes A (hash recorded).
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 0)

        # Cycle 2: new bytes B, failing driver => counter 1, hash stays A.
        png_a = self.server.png_bytes
        self.server.png_bytes = make_test_png(color=(200, 0, 0))
        self.install_epd(FailingEPD(fail_on="display"))
        self.client.process_refresh_cycle()
        self.assertEqual(self.client._consecutive_hw_failures, 1)

        # Cycle 3: bytes A again, working driver => skip (A is already on the
        # panel, E5.2); the skip changes the counter in NO direction.
        self.server.png_bytes = png_a
        self.epd = CountingEPD(artifact_path=self.artifact_path)
        self.client.process_refresh_cycle()
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped"])
        self.assertEqual(self.client._consecutive_hw_failures, 1)

    def test_limit_zero_never_escalates(self):
        """AC4a: HW_FAILURE_LIMIT=0 => 10 failure cycles, recovery runs, no exit."""
        with patch.object(self.config, "HW_FAILURE_LIMIT", 0):
            self.install_epd(FailingEPD(fail_on="init"))
            for _ in range(10):
                self.client.process_refresh_cycle()

        self.assertEqual(self.client._consecutive_hw_failures, 10)
        self.assertEqual(self.heartbeat_statuses(), [])
        # Recovery per cycle keeps running: cycles 2..10 re-load the driver.
        self.assertEqual(self.load_mock.call_count, 9)

    def test_preview_only_never_escalates(self):
        """AC4b: ImportError => preview-only, counter 0, no re-load spam, no exit."""
        client = self.client
        # Real load path with a simulated missing waveshare_epd package:
        # None in sys.modules makes 'from waveshare_epd import ...' raise
        # ImportError deterministically on any machine.
        with patch.dict(sys.modules, {"waveshare_epd": None}):
            with self.assertLogs("eink-client", level="WARNING"):
                self.real_load_display_driver("epd7in3e")

        self.assertIsNone(client.epd)
        self.assertTrue(client._preview_only)
        self.assertFalse(client._hw_recovery_pending)

        with patch.object(Image.Image, "save"):
            for _ in range(5):
                client.process_refresh_cycle()

        self.assertEqual(self.heartbeat_statuses(), ["refreshed"] * 5)
        self.assertEqual(client._consecutive_hw_failures, 0)
        # The load is NOT repeated per cycle (preview-only is permanent).
        self.load_mock.assert_not_called()


class TestInitialRetryAndServerDown(ContentSkipSandbox, unittest.TestCase):
    """E5.4 AC5/AC6: quiet retry without panel calls; initial-write retry."""

    def test_server_down_no_panel_calls_no_escalation(self):
        """AC5: 5 cycles with ConnectionError everywhere - panel untouched."""
        import requests as real_requests

        def raise_connection_error(*args, **kwargs):
            raise real_requests.ConnectionError("server down")

        self.client._server_get.side_effect = raise_connection_error
        self.client._server_post.side_effect = raise_connection_error

        for _ in range(5):
            self.client.process_refresh_cycle()

        self.assertEqual(self.epd.init_calls, 0)
        self.assertEqual(self.epd.display_calls, 0)
        self.assertEqual(self.epd.sleep_calls, 0)
        self.assertEqual(self.client._consecutive_hw_failures, 0)
        self.assertEqual(self.heartbeat_statuses(), [])

        # Server back: the next cycle writes normally (process state intact).
        self.client._server_get.side_effect = self.server.get
        self.client._server_post.side_effect = self.server.post
        self.client.process_refresh_cycle()
        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])

    def test_initial_retry_until_first_success(self):
        """AC6: a failed startup write is retried every poll cycle despite
        should_refresh=false until the first success; then retry mode ends."""
        import requests as real_requests
        client = self.client
        client._initial_display_done = False
        self.server.should_refresh = False

        # Cycle 1: /preview unreachable - no write, no counter, still pending.
        real_get = self.server.get

        def flaky_get(path, timeout=None):
            if path == "/preview":
                raise real_requests.ConnectionError("server still booting")
            return real_get(path, timeout=timeout)

        client._server_get.side_effect = flaky_get
        client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 0)
        self.assertFalse(client._initial_display_done)
        self.assertEqual(client._consecutive_hw_failures, 0)

        # Cycle 2: server back - the panel is written ALTHOUGH should_refresh
        # stays false (initial retry, decision 7).
        client._server_get.side_effect = real_get
        client.process_refresh_cycle()

        self.assertEqual(self.epd.display_calls, 1)
        self.assertTrue(client._initial_display_done)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])

        # Counter-check: retry mode is over - further should_refresh=false
        # cycles never touch the panel or fetch the preview again.
        client.process_refresh_cycle()
        self.assertEqual(self.epd.display_calls, 1)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])
        get_paths = [c.args[0] for c in client._server_get.call_args_list]
        self.assertEqual(get_paths.count("/preview"), 2)


class TestMainLoopRecovery(unittest.TestCase):
    """E5.4: main() runs cleanup() even when the escalation raises SystemExit."""

    def setUp(self):
        import client
        self.addCleanup(
            setattr, client, "_initial_display_done", client._initial_display_done
        )

    @patch("client.cleanup")
    @patch("client.process_refresh_cycle", side_effect=SystemExit(1))
    @patch("client.send_heartbeat")
    @patch("client.fetch_preview", return_value=None)
    @patch("client.fetch_display_config", return_value={})
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_cleanup_runs_on_system_exit(self, mock_time, mock_signal, mock_config,
                                         mock_load, mock_fetch_config,
                                         mock_fetch_preview, mock_heartbeat,
                                         mock_cycle, mock_cleanup):
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 1

        import client
        with self.assertRaises(SystemExit) as cm:
            client.main()

        self.assertEqual(cm.exception.code, 1)
        mock_cycle.assert_called_once()
        mock_cleanup.assert_called_once()


class TestHwFailureLimitConfig(unittest.TestCase):
    """E5.4 AC8: config.HW_FAILURE_LIMIT default value and env override."""

    def tearDown(self):
        # Restore module state from the real environment after reload tests.
        import config
        importlib.reload(config)

    def test_hw_failure_limit_default(self):
        """Without EINK_HW_FAILURE_LIMIT the limit defaults to 3."""
        import config
        with patch.dict(os.environ):
            os.environ.pop("EINK_HW_FAILURE_LIMIT", None)
            importlib.reload(config)
            self.assertEqual(config.HW_FAILURE_LIMIT, 3)

    def test_hw_failure_limit_env_override(self):
        """EINK_HW_FAILURE_LIMIT overrides the default (0 = escalation off)."""
        import config
        with patch.dict(os.environ, {"EINK_HW_FAILURE_LIMIT": "0"}):
            importlib.reload(config)
            self.assertEqual(config.HW_FAILURE_LIMIT, 0)
        with patch.dict(os.environ, {"EINK_HW_FAILURE_LIMIT": "5"}):
            importlib.reload(config)
            self.assertEqual(config.HW_FAILURE_LIMIT, 5)


class TestContentSkipConfig(unittest.TestCase):
    """E5.2 AC11: config.CONTENT_SKIP / config.MAX_SKIP_HOURS defaults and overrides."""

    def tearDown(self):
        # Restore module state from the real environment after reload tests.
        import config
        importlib.reload(config)

    def test_content_skip_default_enabled(self):
        """Without EINK_CONTENT_SKIP the content skip is enabled."""
        import config
        with patch.dict(os.environ):
            os.environ.pop("EINK_CONTENT_SKIP", None)
            importlib.reload(config)
            self.assertTrue(config.CONTENT_SKIP)

    def test_content_skip_only_string_false_disables(self):
        """Only the string "false" (case-insensitive) disables the skip."""
        import config
        cases = [
            ("false", False),
            ("FALSE", False),
            ("False", False),
            ("true", True),
            ("0", True),
            ("no", True),
            ("off", True),
            ("", True),
        ]
        for value, expected in cases:
            with self.subTest(value=value):
                with patch.dict(os.environ, {"EINK_CONTENT_SKIP": value}):
                    importlib.reload(config)
                    self.assertEqual(config.CONTENT_SKIP, expected)

    def test_max_skip_hours_default(self):
        """Without EINK_MAX_SKIP_HOURS the guard defaults to 24 hours."""
        import config
        with patch.dict(os.environ):
            os.environ.pop("EINK_MAX_SKIP_HOURS", None)
            importlib.reload(config)
            self.assertEqual(config.MAX_SKIP_HOURS, 24)

    def test_max_skip_hours_env_override(self):
        """EINK_MAX_SKIP_HOURS overrides the default after module reload."""
        import config
        with patch.dict(os.environ, {"EINK_MAX_SKIP_HOURS": "0"}):
            importlib.reload(config)
            self.assertEqual(config.MAX_SKIP_HOURS, 0)
        with patch.dict(os.environ, {"EINK_MAX_SKIP_HOURS": "48"}):
            importlib.reload(config)
            self.assertEqual(config.MAX_SKIP_HOURS, 48)


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
    def test_send_heartbeat_skipped_status(self, mock_config, mock_requests):
        """E5.2: heartbeat carries an explicit "skipped" status on content skip."""
        mock_config.SERVER_URL = "http://localhost:5000"

        import client
        client.send_heartbeat("skipped")

        mock_requests.post.assert_called_once()
        body = mock_requests.post.call_args[1]["json"]
        self.assertEqual(body["status"], "skipped")
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

    def _mock_settings(self, mock_config, mock_requests, settings):
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        import client
        client.driver_name = "epd7in3e"
        mock_resp = MagicMock()
        mock_resp.ok = True
        mock_resp.json.return_value = settings
        mock_requests.get.return_value = mock_resp

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_config_surfaces_top_level_panel_image_mode(
        self, mock_config, mock_requests
    ):
        """F10 AC5: the TOP-LEVEL panel_image_mode is surfaced into the dict."""
        self._mock_settings(
            mock_config,
            mock_requests,
            {"display": {"driver": "epd7in3e"}, "panel_image_mode": "original"},
        )

        import client
        result = client.fetch_display_config()

        self.assertEqual(result["panel_image_mode"], "original")

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_config_panel_image_mode_missing_defaults_dithered(
        self, mock_config, mock_requests
    ):
        """F10 robustness: a server without the field yields dithered."""
        self._mock_settings(
            mock_config, mock_requests, {"display": {"driver": "epd7in3e"}}
        )

        import client
        result = client.fetch_display_config()

        self.assertEqual(result["panel_image_mode"], "dithered")

    @patch("client.requests")
    @patch("client.config")
    def test_fetch_config_panel_image_mode_unknown_defaults_dithered(
        self, mock_config, mock_requests
    ):
        """F10 robustness: an unexpected value is normalized to dithered."""
        self._mock_settings(
            mock_config,
            mock_requests,
            {"display": {"driver": "epd7in3e"}, "panel_image_mode": "raw"},
        )

        import client
        result = client.fetch_display_config()

        self.assertEqual(result["panel_image_mode"], "dithered")


class TestMainLoop(unittest.TestCase):
    """Test main loop behavior."""

    @patch("client.process_refresh_cycle", return_value=False)
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
                            mock_display, mock_heartbeat, mock_cycle):
        """Main loop performs initial display update on startup."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 30

        test_img = Image.new("RGB", (800, 480), (255, 255, 255))
        mock_fetch_preview.return_value = test_img

        # process_refresh_cycle is stubbed to "poll failed" so the loop takes
        # the bounded backoff path; fake_sleep then breaks out after the
        # initial update. (B3: the loop polls immediately, so the poll must be
        # mocked here to keep the test off the real network.)
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

    @patch("client.process_refresh_cycle", return_value=False)
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
                                  mock_display, mock_heartbeat, mock_cycle):
        """Main loop handles missing image gracefully."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 30

        # Stub the poll to "failed" so the loop backs off; fake_sleep then
        # breaks out (B3: the loop polls immediately, keep it off the network).
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


class TestClientTokenConfig(unittest.TestCase):
    """E5.1 AC10: config.CLIENT_TOKEN default value and env override."""

    def tearDown(self):
        # Restore module state from the real environment after reload tests.
        import config
        importlib.reload(config)

    def test_client_token_default_empty(self):
        """Without EINK_CLIENT_TOKEN the token defaults to empty string."""
        import config
        with patch.dict(os.environ):
            os.environ.pop("EINK_CLIENT_TOKEN", None)
            importlib.reload(config)
            self.assertEqual(config.CLIENT_TOKEN, "")

    def test_client_token_env_override(self):
        """EINK_CLIENT_TOKEN overrides the default after module reload."""
        import config
        with patch.dict(os.environ, {"EINK_CLIENT_TOKEN": "deadbeef01"}):
            importlib.reload(config)
            self.assertEqual(config.CLIENT_TOKEN, "deadbeef01")


class TestClientTokenAuth(unittest.TestCase):
    """E5.1 AC10: X-Client-Token header on all four server calls + 401 handling."""

    TOKEN = "test-token-0123456789abcdef"

    def setUp(self):
        import client
        import config
        self.client = client
        self.config = config
        client._auth_error_logged = False
        self.addCleanup(setattr, client, "_auth_error_logged", False)

    def _patch_config(self, token):
        for name, value in (("CLIENT_TOKEN", token),
                            ("SERVER_URL", "http://localhost:5000")):
            patcher = patch.object(self.config, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)

    @staticmethod
    def _wire_exceptions(mock_requests):
        """Give the requests mock real exception classes for except clauses."""
        import requests as real_requests
        mock_requests.ConnectionError = real_requests.ConnectionError
        mock_requests.HTTPError = real_requests.HTTPError

    def _mock_response(self, status_code=200, json_data=None):
        resp = MagicMock()
        resp.status_code = status_code
        resp.ok = status_code < 400
        resp.json.return_value = json_data if json_data is not None else {}
        resp.content = make_test_png()
        if status_code >= 400:
            import requests as real_requests
            resp.raise_for_status.side_effect = real_requests.HTTPError(
                f"{status_code} Client Error"
            )
        else:
            resp.raise_for_status = MagicMock()
        return resp

    def _call_all_four(self):
        """Hit all four server endpoints once."""
        self.client.fetch_display_config()
        self.client.fetch_preview()
        self.client.check_should_refresh()
        self.client.send_heartbeat()

    @patch("client.load_display_driver")
    @patch("client.requests")
    def test_header_sent_on_all_four_calls(self, mock_requests, mock_load):
        """With a configured token every call carries X-Client-Token."""
        self._patch_config(self.TOKEN)
        self._wire_exceptions(mock_requests)
        mock_requests.get.return_value = self._mock_response(
            json_data={"display": {}, "should_refresh": False}
        )
        mock_requests.post.return_value = self._mock_response()

        self._call_all_four()

        get_calls = mock_requests.get.call_args_list
        self.assertEqual(len(get_calls), 3)
        get_urls = [call.args[0] for call in get_calls]
        self.assertIn("http://localhost:5000/settings", get_urls)
        self.assertIn("http://localhost:5000/preview", get_urls)
        self.assertIn("http://localhost:5000/api/refresh_status", get_urls)
        for call in get_calls:
            self.assertEqual(
                call.kwargs["headers"], {"X-Client-Token": self.TOKEN}
            )

        post_call = mock_requests.post.call_args
        self.assertIn("/api/client_heartbeat", post_call.args[0])
        self.assertEqual(
            post_call.kwargs["headers"], {"X-Client-Token": self.TOKEN}
        )

    @patch("client.load_display_driver")
    @patch("client.requests")
    def test_no_header_with_empty_token(self, mock_requests, mock_load):
        """Empty token = no X-Client-Token header (today's behavior)."""
        self._patch_config("")
        self._wire_exceptions(mock_requests)
        mock_requests.get.return_value = self._mock_response(
            json_data={"display": {}, "should_refresh": False}
        )
        mock_requests.post.return_value = self._mock_response()

        self._call_all_four()

        all_calls = mock_requests.get.call_args_list + mock_requests.post.call_args_list
        self.assertEqual(len(all_calls), 4)
        for call in all_calls:
            headers = call.kwargs.get("headers") or {}
            self.assertNotIn("X-Client-Token", headers)

    @patch("client.requests")
    def test_401_logs_error_once_no_crash(self, mock_requests):
        """401 logs one clear EINK_CLIENT_TOKEN hint, calls keep returning safely."""
        self._patch_config(self.TOKEN)
        self._wire_exceptions(mock_requests)
        mock_requests.get.return_value = self._mock_response(status_code=401)
        mock_requests.post.return_value = self._mock_response(status_code=401)

        with self.assertLogs("eink-client", level="DEBUG") as logs:
            # Simulated poll loop: repeated polls plus the other endpoints.
            self.assertFalse(self.client.check_should_refresh())
            self.assertFalse(self.client.check_should_refresh())
            self.assertFalse(self.client.check_should_refresh())
            self.assertEqual(self.client.fetch_display_config(), {})
            self.assertIsNone(self.client.fetch_preview())
            self.client.send_heartbeat()  # must not raise

        auth_errors = [
            r for r in logs.records
            if r.levelno == logging.ERROR and "EINK_CLIENT_TOKEN" in r.getMessage()
        ]
        self.assertEqual(
            len(auth_errors), 1,
            f"expected exactly one auth error log, got: {logs.output}",
        )
        message = auth_errors[0].getMessage()
        self.assertIn("401", message)
        self.assertIn(".env", message)

    @patch("client.requests")
    def test_401_logged_again_after_recovery(self, mock_requests):
        """The hint fires once per state change: 401 -> ok -> 401 logs twice."""
        self._patch_config(self.TOKEN)
        self._wire_exceptions(mock_requests)
        ok_resp = self._mock_response(json_data={"should_refresh": False})
        bad_resp = self._mock_response(status_code=401)

        with self.assertLogs("eink-client", level="DEBUG") as logs:
            mock_requests.get.return_value = bad_resp
            self.client.check_should_refresh()
            self.client.check_should_refresh()
            mock_requests.get.return_value = ok_resp
            self.client.check_should_refresh()
            mock_requests.get.return_value = bad_resp
            self.client.check_should_refresh()

        auth_errors = [
            r for r in logs.records
            if r.levelno == logging.ERROR and "EINK_CLIENT_TOKEN" in r.getMessage()
        ]
        self.assertEqual(len(auth_errors), 2, f"got: {logs.output}")

    @patch("client.cleanup")
    @patch("client.load_display_driver")
    @patch("client.requests")
    @patch("client.signal")
    @patch("client.time")
    def test_main_loop_survives_401(self, mock_time, mock_signal, mock_requests,
                                    mock_load, mock_cleanup):
        """Main loop keeps polling through persistent 401 responses."""
        import requests as real_requests
        self._patch_config(self.TOKEN)
        poll_patcher = patch.object(self.config, "POLL_INTERVAL", 2)
        poll_patcher.start()
        self.addCleanup(poll_patcher.stop)

        mock_requests.get.return_value = self._mock_response(status_code=401)
        mock_requests.post.return_value = self._mock_response(status_code=401)
        mock_requests.ConnectionError = real_requests.ConnectionError

        # Deterministic initial-retry path: no hardware, no pending recovery,
        # first frame still pending (so the loop reaches the poll each round).
        for attr, value in (("epd", None), ("_hw_recovery_pending", False),
                            ("_consecutive_hw_failures", 0),
                            ("_initial_display_done", False)):
            self.addCleanup(setattr, self.client, attr, getattr(self.client, attr))
            setattr(self.client, attr, value)

        # B3 loop: poll first, then a bounded backoff on the (401 -> {}) poll.
        # POLL_INTERVAL=2 -> 2 backoff sleeps/round; break on the 3rd sleep so
        # exactly two refresh_status polls happen (round 1 + round 2).
        call_count = [0]

        def fake_sleep(seconds):
            call_count[0] += 1
            if call_count[0] >= 3:
                raise KeyboardInterrupt()

        mock_time.sleep.side_effect = fake_sleep
        mock_time.strftime = time.strftime
        mock_time.gmtime = time.gmtime

        with self.assertLogs("eink-client", level="DEBUG") as logs:
            try:
                self.client.main()
            except KeyboardInterrupt:
                pass

        # Startup (settings + preview) plus two refresh_status polls: the
        # loop kept running after the first 401 instead of crashing.
        get_urls = [call.args[0] for call in mock_requests.get.call_args_list]
        self.assertEqual(
            len([u for u in get_urls if u.endswith("/api/refresh_status")]), 2
        )
        auth_errors = [
            r for r in logs.records
            if r.levelno == logging.ERROR and "EINK_CLIENT_TOKEN" in r.getMessage()
        ]
        self.assertEqual(
            len(auth_errors), 1,
            f"expected exactly one auth error log, got: {logs.output}",
        )

    @patch("client.load_display_driver")
    @patch("client.requests")
    def test_token_value_never_logged(self, mock_requests, mock_load):
        """Secrets hygiene: the token value must not appear in any log record."""
        secret = "super-secret-token-value-a1b2c3d4"
        self._patch_config(secret)
        self._wire_exceptions(mock_requests)
        mock_requests.get.return_value = self._mock_response(status_code=401)
        mock_requests.post.return_value = self._mock_response(status_code=401)

        with self.assertLogs("eink-client", level="DEBUG") as logs:
            self._call_all_four()

        for record in logs.records:
            self.assertNotIn(secret, record.getMessage())


class TestLongPollManualRefresh(ContentSkipSandbox, unittest.TestCase):
    """B3 AC19: a manual refresh status triggers exactly one non-skipped write."""

    def test_manual_status_single_refresh_never_skipped(self):
        """AC19: should_refresh=true + reason="manual" writes exactly once and
        is never content-skipped, even when the bytes are identical to the last
        frame, and threads reason="manual" through process_refresh_cycle ->
        handle_refresh."""
        # Cycle 1 records the hash, so identical content WOULD be skipped for an
        # interval-driven refresh (the B6 content skip).
        self.client.process_refresh_cycle()
        self.assertEqual(self.epd.display_calls, 1)

        # Cycle 2: identical bytes, but the server now reports a manual trigger.
        self.server.reason = "manual"
        with patch.object(
            self.client, "handle_refresh", wraps=self.client.handle_refresh
        ) as spy:
            self.client.process_refresh_cycle()

        # Exactly one refresh, with reason threaded positionally as
        # handle_refresh(display_config, reason).
        spy.assert_called_once()
        self.assertEqual(spy.call_args.args[1], "manual")
        # Wrote again instead of skipping (manual is never content-skipped).
        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])


class TestLongPollLoop(unittest.TestCase):
    """B3 AC15/AC20: immediate re-poll on success, bounded backoff on failure."""

    def setUp(self):
        import client
        self.client = client
        # Deterministic startup: no hardware, no pending recovery, clean
        # counters (the loop's poll is mocked, so nothing touches the panel).
        for attr, value in (("epd", None), ("_hw_recovery_pending", False),
                            ("_consecutive_hw_failures", 0),
                            ("_initial_display_done", False)):
            self.addCleanup(setattr, client, attr, getattr(client, attr))
            setattr(client, attr, value)

    @patch("client.cleanup")
    @patch("client.process_refresh_cycle")
    @patch("client.fetch_preview", return_value=None)
    @patch("client.fetch_display_config", return_value={})
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_poll_failure_triggers_bounded_backoff(self, mock_time, mock_signal,
                                                   mock_config, mock_load,
                                                   mock_fetch_config,
                                                   mock_fetch_preview, mock_cycle,
                                                   mock_cleanup):
        """AC20: a failed poll (network error / timeout / non-2xx) makes the
        loop back off POLL_INTERVAL one-second sleeps instead of busy-looping."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 3

        cycles = [0]

        def failing_cycle():
            cycles[0] += 1
            if cycles[0] >= 3:
                raise KeyboardInterrupt()
            return False  # poll failed -> back off, do NOT re-poll tightly

        mock_cycle.side_effect = failing_cycle

        with self.assertRaises(KeyboardInterrupt):
            self.client.main()

        # Two completed cycles, each followed by 3 one-second backoff sleeps
        # (== 6 sleeps): the loop paced itself instead of spinning.
        self.assertEqual(mock_cycle.call_count, 3)
        self.assertEqual(mock_time.sleep.call_count, 6)
        for call in mock_time.sleep.call_args_list:
            self.assertEqual(call.args, (1,))

    @patch("client.cleanup")
    @patch("client.process_refresh_cycle")
    @patch("client.fetch_preview", return_value=None)
    @patch("client.fetch_display_config", return_value={})
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_successful_poll_repolls_without_backoff(self, mock_time, mock_signal,
                                                     mock_config, mock_load,
                                                     mock_fetch_config,
                                                     mock_fetch_preview, mock_cycle,
                                                     mock_cleanup):
        """AC15: a successful poll re-polls immediately - no happy-path sleep
        (the server's long-poll hold provides the pacing)."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 30

        cycles = [0]

        def ok_cycle():
            cycles[0] += 1
            if cycles[0] >= 4:
                raise KeyboardInterrupt()
            return True  # poll ok -> immediate re-poll, no backoff sleep

        mock_cycle.side_effect = ok_cycle

        with self.assertRaises(KeyboardInterrupt):
            self.client.main()

        self.assertEqual(mock_cycle.call_count, 4)
        mock_time.sleep.assert_not_called()


class TestProgressGating(ContentSkipSandbox, unittest.TestCase):
    """B3 round 2: process_refresh_cycle() gates the immediate re-poll on
    forward progress (a heartbeat). should_refresh=true is answered by the
    server immediately (the ~25s hold only covers should_refresh=false), so a
    due-but-stuck cycle that re-polled at once would busy-loop; it must back off.
    """

    def test_manual_success_signals_immediate_repoll(self):
        """A successful manual refresh made progress -> re-poll immediately
        (preserves the B3 latency win for the happy manual path)."""
        # Prime the panel so identical bytes would otherwise be skippable.
        self.assertTrue(self.client.process_refresh_cycle())
        self.server.reason = "manual"
        # Manual write succeeds -> heartbeat "refreshed" -> immediate re-poll.
        self.assertTrue(self.client.process_refresh_cycle())
        self.assertEqual(self.epd.display_calls, 2)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "refreshed"])

    def test_manual_write_failure_signals_backoff(self):
        """Round 2 case 2: a manual refresh whose panel write FAILS makes no
        progress (no heartbeat) -> back off. Without this the hw-failure counter
        would race to SystemExit in a tight 0ms loop (systemd restart storm)."""
        self.client.process_refresh_cycle()  # cycle 1 writes, records hash
        self.server.reason = "manual"
        with patch.object(self.epd, "display", side_effect=Exception("SPI error")):
            repoll = self.client.process_refresh_cycle()
        self.assertFalse(repoll)  # no progress -> caller must back off
        # The failed cycle sent no heartbeat and bumped the counter once only.
        self.assertEqual(self.heartbeat_statuses(), ["refreshed"])
        self.assertEqual(self.client._consecutive_hw_failures, 1)

    def test_content_skip_signals_immediate_repoll(self):
        """The content-skip path sends a "skipped" heartbeat -> progress ->
        immediate re-poll (the skip path must stay fast); the server hold then
        paces the following poll."""
        self.client.process_refresh_cycle()  # cycle 1 writes
        repoll = self.client.process_refresh_cycle()  # cycle 2 identical -> skip
        self.assertTrue(repoll)
        self.assertEqual(self.heartbeat_statuses(), ["refreshed", "skipped"])

    def test_no_refresh_due_signals_immediate_repoll(self):
        """should_refresh=false means the server already held ~25s -> re-poll
        immediately (nothing due, no heartbeat needed, no busy-loop)."""
        self.server.should_refresh = False
        self.assertTrue(self.client.process_refresh_cycle())
        self.assertEqual(self.heartbeat_statuses(), [])

    def test_initial_retry_no_image_signals_backoff(self):
        """Fresh boot, refresh_status is 2xx but /preview is unreachable: no
        image -> no heartbeat -> back off (the unbounded-spin case, unit level)."""
        import requests as real_requests
        client = self.client
        client._initial_display_done = False
        real_get = self.server.get

        def flaky_get(path, timeout=None):
            if path == "/preview":
                raise real_requests.ConnectionError("server still booting")
            return real_get(path, timeout=timeout)

        client._server_get.side_effect = flaky_get
        repoll = client.process_refresh_cycle()

        self.assertFalse(repoll)  # no progress -> back off, do not spin
        self.assertEqual(self.epd.display_calls, 0)
        self.assertEqual(self.heartbeat_statuses(), [])


class TestLongPollNoProgressBackoff(unittest.TestCase):
    """B3 round 2 regression: the REAL loop (process_refresh_cycle NOT stubbed)
    must pace a due refresh that makes no forward progress, never busy-loop."""

    def setUp(self):
        import client
        self.client = client
        for attr, value in (("epd", None), ("_hw_recovery_pending", False),
                            ("_consecutive_hw_failures", 0),
                            ("_initial_display_done", False)):
            self.addCleanup(setattr, client, attr, getattr(client, attr))
            setattr(client, attr, value)

    @patch("client.cleanup")
    @patch("client.send_heartbeat")
    @patch("client.fetch_preview", return_value=None)
    @patch("client.fetch_display_config", return_value={})
    @patch("client.get_refresh_status")
    @patch("client.load_display_driver")
    @patch("client.config")
    @patch("client.signal")
    @patch("client.time")
    def test_fresh_boot_no_image_does_not_busyloop(self, mock_time, mock_signal,
                                                   mock_config, mock_load,
                                                   mock_status, mock_fetch_config,
                                                   mock_fetch_preview,
                                                   mock_heartbeat, mock_cleanup):
        """Fresh boot: the server reports should_refresh=true (answered at once,
        no hold) but no image is available yet (fetch_preview -> None). Here
        process_refresh_cycle runs FOR REAL, so dropping the no-progress backoff
        turns this into a 0ms spin: time.sleep never fires and the safety cap
        trips. With the fix the loop backs off POLL_INTERVAL between polls ->
        bounded polls, no spin, no heartbeat."""
        mock_config.DISPLAY_DRIVER = "epd7in3e"
        mock_config.SERVER_URL = "http://localhost:5000"
        mock_config.POLL_INTERVAL = 3

        status_calls = [0]

        def status_side_effect(*args, **kwargs):
            status_calls[0] += 1
            # Safety net: a no-progress spin (fix removed) calls this unboundedly
            # with zero sleeps -> fail loudly instead of hanging the suite.
            if status_calls[0] > 50:
                raise AssertionError(
                    "busy-loop: re-polled without backoff on a no-progress cycle"
                )
            return {"should_refresh": True, "reason": "interval"}

        mock_status.side_effect = status_side_effect

        sleeps = [0]

        def fake_sleep(seconds):
            sleeps[0] += 1
            if sleeps[0] >= 6:
                raise KeyboardInterrupt()

        mock_time.sleep.side_effect = fake_sleep
        mock_time.strftime = time.strftime
        mock_time.gmtime = time.gmtime

        with self.assertRaises(KeyboardInterrupt):
            self.client.main()

        # No image was ever available -> no heartbeat -> no forward progress.
        mock_heartbeat.assert_not_called()
        # Paced: 6 one-second sleeps over 2 backoff rounds (POLL_INTERVAL=3),
        # so only ~2 status polls happened - bounded, not a 0ms spin.
        self.assertLessEqual(status_calls[0], 3)
        self.assertEqual(mock_time.sleep.call_count, 6)
        for call in mock_time.sleep.call_args_list:
            self.assertEqual(call.args, (1,))


if __name__ == "__main__":
    unittest.main()
