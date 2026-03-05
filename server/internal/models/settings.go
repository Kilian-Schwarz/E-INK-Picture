package models

// Settings holds application-wide configuration persisted to disk.
type Settings struct {
	DisplayType DisplayType `json:"display_type"`
}

// SettingsResponse is the API response including the resolved display config.
type SettingsResponse struct {
	DisplayType DisplayType   `json:"display_type"`
	Display     DisplayConfig `json:"display"`
}
