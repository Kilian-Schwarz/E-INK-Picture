package models

// RenderQuality controls the rendering pipeline quality vs speed tradeoff.
type RenderQuality string

const (
	RenderQualityHigh   RenderQuality = "high"   // 2x supersampling + dithering
	RenderQualityMedium RenderQuality = "medium" // 1.5x supersampling + dithering
	RenderQualityFast   RenderQuality = "fast"   // 1x, no supersampling (legacy)
)

// Settings holds application-wide configuration persisted to disk.
type Settings struct {
	DisplayType        DisplayType   `json:"display_type"`
	RefreshInterval    int           `json:"refresh_interval"`
	RenderQuality      RenderQuality `json:"render_quality,omitempty"`
	LastRefreshTrigger string        `json:"last_refresh_trigger,omitempty"`
	LastClientRefresh  string        `json:"last_client_refresh,omitempty"`
}

// SettingsResponse is the API response including the resolved display config.
type SettingsResponse struct {
	DisplayType     DisplayType   `json:"display_type"`
	Display         DisplayConfig `json:"display"`
	RefreshInterval int           `json:"refresh_interval"`
	RenderQuality   RenderQuality `json:"render_quality"`
}

// RefreshStatus is the response for the refresh status endpoint.
type RefreshStatus struct {
	ShouldRefresh     bool   `json:"should_refresh"`
	RefreshInterval   int    `json:"refresh_interval"`
	LastTrigger       string `json:"last_trigger,omitempty"`
	LastClientRefresh string `json:"last_client_refresh,omitempty"`
}
