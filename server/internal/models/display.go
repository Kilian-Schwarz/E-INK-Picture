package models

// DisplayType identifies a supported e-ink display model.
type DisplayType string

const (
	DisplayWaveshare75V2 DisplayType = "waveshare_7in5_v2"
	DisplayWaveshare73E  DisplayType = "waveshare_7in3_e"
)

// DisplayConfig describes the capabilities of an e-ink display.
type DisplayConfig struct {
	Type       DisplayType `json:"type"`
	Name       string      `json:"name"`
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Colors     []string    `json:"colors"`
	ColorNames []string    `json:"color_names"`
	Driver     string      `json:"driver"`
	RefreshSec int         `json:"refresh_sec"`
}

// DisplayProfiles maps display types to their configuration.
var DisplayProfiles = map[DisplayType]DisplayConfig{
	DisplayWaveshare75V2: {
		Type:       DisplayWaveshare75V2,
		Name:       "Waveshare 7.5\" V2 (Black/White)",
		Width:      800,
		Height:     480,
		Colors:     []string{"#000000", "#FFFFFF"},
		ColorNames: []string{"Black", "White"},
		Driver:     "epd7in5_V2",
		RefreshSec: 5,
	},
	DisplayWaveshare73E: {
		Type:       DisplayWaveshare73E,
		Name:       "Waveshare 7.3\" E (6-Color Spectra 6)",
		Width:      800,
		Height:     480,
		Colors:     []string{"#000000", "#FFFFFF", "#FF0000", "#FFFF00", "#00FF00", "#0000FF"},
		ColorNames: []string{"Black", "White", "Red", "Yellow", "Green", "Blue"},
		Driver:     "epd7in3e",
		RefreshSec: 25,
	},
}

// DisplayProfileList returns all profiles as a sorted slice.
func DisplayProfileList() []DisplayConfig {
	return []DisplayConfig{
		DisplayProfiles[DisplayWaveshare75V2],
		DisplayProfiles[DisplayWaveshare73E],
	}
}

// GetDisplayConfig returns the config for a display type, defaulting to 7.5" V2.
func GetDisplayConfig(t DisplayType) DisplayConfig {
	if cfg, ok := DisplayProfiles[t]; ok {
		return cfg
	}
	return DisplayProfiles[DisplayWaveshare75V2]
}
