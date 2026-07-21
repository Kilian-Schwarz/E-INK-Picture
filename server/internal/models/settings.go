package models

// RenderQuality controls the rendering pipeline quality vs speed tradeoff.
type RenderQuality string

const (
	RenderQualityHigh   RenderQuality = "high"   // 2x supersampling + dithering
	RenderQualityMedium RenderQuality = "medium" // 1.5x supersampling + dithering
	RenderQualityFast   RenderQuality = "fast"   // 1x, no supersampling (legacy)
)

// DitherAlgorithm selects the error-diffusion algorithm used during palette
// quantization.
type DitherAlgorithm string

const (
	DitherFloydSteinberg DitherAlgorithm = "floyd_steinberg" // stdlib draw.FloydSteinberg (default)
	DitherAtkinson       DitherAlgorithm = "atkinson"        // 6-neighbor Atkinson, 1/8 weights
)

// CalibrationMode selects whether the display profile's panel calibration
// (perceptual panel palette + precompensation preset) is applied.
type CalibrationMode string

const (
	CalibrationDefault CalibrationMode = "default" // profile preset active (default)
	CalibrationOff     CalibrationMode = "off"     // ideal driver palette, no precompensation
)

// PanelImageMode selects whether the image sent to the e-ink panel is the
// server-dithered frame (current behaviour) or the unquantized original,
// leaving the palette mapping to the Waveshare driver (specs/F10).
type PanelImageMode string

const (
	PanelImageDithered PanelImageMode = "dithered" // server-dithered, current behaviour (default)
	PanelImageOriginal PanelImageMode = "original" // unquantized original, driver hard-maps palette
)

// NormalizePanelImageMode returns m when it is a known value, otherwise the
// default PanelImageDithered. Unlike the handler's request validation (which
// hard-rejects unknown input), this normalizes hand-edited/corrupt on-disk
// values so a bad settings.json never freezes rendering.
func NormalizePanelImageMode(m PanelImageMode) PanelImageMode {
	switch m {
	case PanelImageOriginal:
		return PanelImageOriginal
	default:
		return PanelImageDithered
	}
}

// Refresh reasons reported in RefreshStatus when should_refresh is true.
const (
	RefreshReasonManual   = "manual"   // manual trigger, breaks the sleep window
	RefreshReasonInterval = "interval" // refresh interval elapsed
)

// Settings holds application-wide configuration persisted to disk.
// SleepStart/SleepEnd define the nightly sleep window in local wall-clock
// time ("HH:MM", 24h); both empty means the window is disabled.
// SetupCompleted is the setup wizard's one-way completion latch
// (specs/E2.3-setup-wizard.md): the wizard sets it on finish or skip; once
// true it never returns to false. Being a struct field it survives the
// TriggerRefresh/RecordClientRefresh roundtrips that rewrite settings.json.
type Settings struct {
	DisplayType        DisplayType     `json:"display_type"`
	RefreshInterval    int             `json:"refresh_interval"`
	RenderQuality      RenderQuality   `json:"render_quality,omitempty"`
	DitherAlgorithm    DitherAlgorithm `json:"dither_algorithm,omitempty"`
	Calibration        CalibrationMode `json:"calibration,omitempty"`
	PanelImageMode     PanelImageMode  `json:"panel_image_mode,omitempty"`
	SleepStart         string          `json:"sleep_start,omitempty"`
	SleepEnd           string          `json:"sleep_end,omitempty"`
	SetupCompleted     bool            `json:"setup_completed,omitempty"`
	LastRefreshTrigger string          `json:"last_refresh_trigger,omitempty"`
	LastClientRefresh  string          `json:"last_client_refresh,omitempty"`
}

// SettingsResponse is the API response including the resolved display config.
type SettingsResponse struct {
	DisplayType     DisplayType     `json:"display_type"`
	Display         DisplayConfig   `json:"display"`
	RefreshInterval int             `json:"refresh_interval"`
	RenderQuality   RenderQuality   `json:"render_quality"`
	DitherAlgorithm DitherAlgorithm `json:"dither_algorithm"`
	Calibration     CalibrationMode `json:"calibration"`
	PanelImageMode  PanelImageMode  `json:"panel_image_mode"`
	SleepStart      string          `json:"sleep_start"`
	SleepEnd        string          `json:"sleep_end"`
}

// RefreshStatus is the response for the refresh status endpoint.
type RefreshStatus struct {
	ShouldRefresh     bool   `json:"should_refresh"`
	Reason            string `json:"reason,omitempty"`
	RefreshInterval   int    `json:"refresh_interval"`
	LastTrigger       string `json:"last_trigger,omitempty"`
	LastClientRefresh string `json:"last_client_refresh,omitempty"`
}
