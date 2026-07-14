# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Client writes the exact panel image (after resize and mode conversion, immediately before the EPD driver call) to `/tmp/eink_last_sent.png` for hardware-in-the-loop verification. Path is configurable via `EINK_LAST_SENT_PATH`. The file is written atomically; a write failure never aborts the display refresh.
- Golden-file render test harness with byte-exact golden comparisons and exact palette assertions for both display profiles (`waveshare_7in3_e`, `waveshare_7in5_v2`). Update golden files with `go test ./internal/services -run TestGoldenRender -update`.
- `EINK_DISPLAY_TYPE` environment variable to configure the server's default display profile. Valid values: `waveshare_7in3_e` (6-color, driver `epd7in3e`) and `waveshare_7in5_v2` (B/W, driver `epd7in5_V2`). Applies only when `settings.json` has no `display_type` value.

### Changed

- Fresh installations (no `settings.json` and no `EINK_DISPLAY_TYPE` set) now default to `waveshare_7in3_e` (7.3" 6-color) instead of `waveshare_7in5_v2` (7.5" B/W). This matches the client's default driver `epd7in3e`. **Migration note:** users of the 7.5" V2 panel without a persisted `settings.json` must set `EINK_DISPLAY_TYPE=waveshare_7in5_v2` or select the display once in the settings UI. Existing `settings.json` values always win and are not affected.
