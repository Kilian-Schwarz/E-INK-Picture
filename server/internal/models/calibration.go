package models

import "log/slog"

// PrecompensationPreset holds the gamma/saturation/contrast precompensation
// applied to the composited image before error diffusion. {1, 1, 1} is the
// identity and skips the pass entirely.
type PrecompensationPreset struct {
	Gamma      float64
	Saturation float64
	Contrast   float64
}

// IsIdentity reports whether the preset leaves every pixel untouched.
func (p PrecompensationPreset) IsIdentity() bool {
	return p.Gamma == 1 && p.Saturation == 1 && p.Contrast == 1
}

// CalibrationProfile describes how a display panel really renders its colors.
// PanelPalette is the perceptual RGB appearance of the driver colors,
// index-aligned to DisplayConfig.Colors — the invariant
// PanelPalette[i] <-> DisplayConfig.Colors[i] is binding: error diffusion
// computes distances against PanelPalette, the output indices are then
// remapped 1:1 onto the pure driver colors.
type CalibrationProfile struct {
	PanelPalette []string
	Precomp      PrecompensationPreset
}

// CalibrationProfiles maps display types to their calibration profile.
//
// The epd7in3e (Spectra 6) values are community-measured appearance values
// (quark-zju gist / epdoptimize, white-point adapted for the Waveshare
// PhotoPainter 7.3" E6 panel) — status: initial guess, tunable at the
// physical panel. Changing them only requires a golden -update commit.
// The 7.5" V2 B/W profile is the identity by design (binding decision).
var CalibrationProfiles = map[DisplayType]CalibrationProfile{
	DisplayWaveshare73E: {
		PanelPalette: []string{"#191E21", "#E8E8E8", "#B21318", "#EFDE44", "#125F20", "#2157BA"},
		Precomp:      PrecompensationPreset{Gamma: 1.0, Saturation: 1.15, Contrast: 1.0},
	},
	DisplayWaveshare75V2: {
		PanelPalette: []string{"#000000", "#FFFFFF"},
		Precomp:      PrecompensationPreset{Gamma: 1.0, Saturation: 1.0, Contrast: 1.0},
	},
}

// GetCalibrationProfile returns the calibration profile for a display type.
// For unknown types or profiles whose PanelPalette length does not match the
// driver palette it returns the identity profile (driver palette, identity
// precompensation) and logs a warning — the quantization pipeline must never
// dither against a palette of a different length than the driver palette,
// or the index mapping back to driver colors would be corrupt.
func GetCalibrationProfile(t DisplayType) CalibrationProfile {
	cfg := GetDisplayConfig(t)
	identity := CalibrationProfile{
		PanelPalette: cfg.Colors,
		Precomp:      PrecompensationPreset{Gamma: 1.0, Saturation: 1.0, Contrast: 1.0},
	}

	profile, ok := CalibrationProfiles[t]
	if !ok {
		slog.Warn("no calibration profile for display type, using identity",
			"display_type", string(t))
		return identity
	}
	if len(profile.PanelPalette) != len(cfg.Colors) {
		slog.Warn("calibration profile palette length mismatch, using identity",
			"display_type", string(t),
			"panel_palette", len(profile.PanelPalette),
			"driver_palette", len(cfg.Colors))
		return identity
	}
	return profile
}
