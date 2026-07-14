# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- New settings `dither_algorithm` (`floyd_steinberg` | `atkinson`, default `floyd_steinberg`) and `calibration` (`default` | `off`, default `default`), persisted in `settings.json`, exposed via `GET /settings` and validated by `POST /update_settings` (unknown values return 400). Configuration is API-only for now (no settings UI yet).
- Atkinson dithering as an alternative error-diffusion algorithm (integer-only, 6 neighbors at 1/8 weight each — higher local contrast and brighter highlights, often a calmer image on e-ink).
- Panel calibration profiles (`server/internal/models/calibration.go`): a perceptual Spectra-6 panel palette (community-measured appearance values, white-point adapted) plus a per-profile gamma/saturation/contrast precompensation preset. Values are an initial guess and tunable at the physical panel without format changes.
- Deterministic calibration test design (`server/internal/services/testdata/designs/calibration.json`): 6 driver-color swatches, a 16-step gray ramp, skin-tone and mixed-color patches, and a text matrix for on-panel A/B evaluation via `POST /api/designs` → activate → `POST /api/trigger_refresh`.

### Changed

- **Calibrated dithering is now the default for the 6-color panel (`waveshare_7in3_e`):** error diffusion runs against the measured panel appearance of the six Spectra-6 colors (plus a mild saturation precompensation) and maps the result index-preserving back onto the pure driver colors. The PNG sent to the client still contains exclusively the 6 ideal driver colors — client and driver are unchanged — but the rendered dithering pattern (and thus the panel image) changes visibly. **Escape hatch:** `POST /update_settings` with `{"calibration":"off"}` restores the previous output byte-exactly. The B/W profile (`waveshare_7in5_v2`) uses an identity profile and is completely unaffected.

- The Go renderer now honors `Element.Rotation` (degrees, clockwise, around the element's top-left anchor — matching the Fabric.js designer semantics). Rotated elements render correctly on the panel and in the server-rendered live previews; anti-aliased rotated edges are dithered before palette quantization. Exact multiples of 90° stay pixel-exact axis-parallel, and element culling accounts for the rotated bounding box.
- Client writes the exact panel image (after resize and mode conversion, immediately before the EPD driver call) to `/tmp/eink_last_sent.png` for hardware-in-the-loop verification. Path is configurable via `EINK_LAST_SENT_PATH`. The file is written atomically; a write failure never aborts the display refresh.
- Golden-file render test harness with byte-exact golden comparisons and exact palette assertions for both display profiles (`waveshare_7in3_e`, `waveshare_7in5_v2`). Update golden files with `go test ./internal/services -run TestGoldenRender -update`.
- `EINK_DISPLAY_TYPE` environment variable to configure the server's default display profile. Valid values: `waveshare_7in3_e` (6-color, driver `epd7in3e`) and `waveshare_7in5_v2` (B/W, driver `epd7in5_V2`). Applies only when `settings.json` has no `display_type` value.

### Changed

- Fresh installations (no `settings.json` and no `EINK_DISPLAY_TYPE` set) now default to `waveshare_7in3_e` (7.3" 6-color) instead of `waveshare_7in5_v2` (7.5" B/W). This matches the client's default driver `epd7in3e`. **Migration note:** users of the 7.5" V2 panel without a persisted `settings.json` must set `EINK_DISPLAY_TYPE=waveshare_7in5_v2` or select the display once in the settings UI. Existing `settings.json` values always win and are not affected.
