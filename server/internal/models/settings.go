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

// Refresh reasons reported in RefreshStatus when should_refresh is true.
const (
	RefreshReasonManual   = "manual"   // manual trigger, breaks the sleep window
	RefreshReasonInterval = "interval" // refresh interval elapsed
)

// Settings holds application-wide configuration persisted to disk.
// SleepStart/SleepEnd define the nightly sleep window in local wall-clock
// time ("HH:MM", 24h); both empty means the window is disabled.
type Settings struct {
	DisplayType        DisplayType     `json:"display_type"`
	RefreshInterval    int             `json:"refresh_interval"`
	RenderQuality      RenderQuality   `json:"render_quality,omitempty"`
	DitherAlgorithm    DitherAlgorithm `json:"dither_algorithm,omitempty"`
	Calibration        CalibrationMode `json:"calibration,omitempty"`
	SleepStart         string          `json:"sleep_start,omitempty"`
	SleepEnd           string          `json:"sleep_end,omitempty"`
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
