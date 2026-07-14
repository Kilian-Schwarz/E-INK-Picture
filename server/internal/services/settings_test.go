package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"e-ink-picture/server/internal/models"
)

func TestSettingsService_DefaultRefreshInterval(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	settings, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.RefreshInterval != defaultRefreshInterval {
		t.Errorf("expected default refresh interval %d, got %d", defaultRefreshInterval, settings.RefreshInterval)
	}
}

func TestSettingsService_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	settings, _ := svc.GetSettings()
	settings.RefreshInterval = 900
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	loaded, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after save: %v", err)
	}
	if loaded.RefreshInterval != 900 {
		t.Errorf("expected refresh interval 900, got %d", loaded.RefreshInterval)
	}
}

func TestSettingsService_TriggerRefresh(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	ts, err := svc.TriggerRefresh()
	if err != nil {
		t.Fatalf("TriggerRefresh: %v", err)
	}
	if ts == "" {
		t.Error("expected non-empty timestamp")
	}

	settings, _ := svc.GetSettings()
	if settings.LastRefreshTrigger != ts {
		t.Errorf("expected last_refresh_trigger=%q, got %q", ts, settings.LastRefreshTrigger)
	}
}

func TestSettingsService_RefreshStatus_ShouldRefreshOnTrigger(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	// Write settings with a past client refresh directly to avoid time.Now() in RecordClientRefresh
	pastTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	s := models.Settings{
		DisplayType:       models.DisplayWaveshare75V2,
		RefreshInterval:   defaultRefreshInterval,
		LastClientRefresh: pastTime,
	}
	data, _ := json.MarshalIndent(&s, "", "  ")
	os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644)

	// Trigger refresh (now)
	if _, err := svc.TriggerRefresh(); err != nil {
		t.Fatalf("TriggerRefresh: %v", err)
	}

	status, err := svc.GetRefreshStatus()
	if err != nil {
		t.Fatalf("GetRefreshStatus: %v", err)
	}
	if !status.ShouldRefresh {
		t.Error("expected should_refresh=true after trigger")
	}
}

func TestSettingsService_RefreshStatus_ShouldNotRefreshRecently(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	// Record a very recent client refresh
	now := time.Now().UTC().Format(time.RFC3339)
	if err := svc.RecordClientRefresh(now); err != nil {
		t.Fatalf("RecordClientRefresh: %v", err)
	}

	status, err := svc.GetRefreshStatus()
	if err != nil {
		t.Fatalf("GetRefreshStatus: %v", err)
	}
	if status.ShouldRefresh {
		t.Error("expected should_refresh=false when just refreshed")
	}
}

func TestSettingsService_RefreshStatus_IntervalElapsed(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	// Write settings with short interval and a past client refresh directly
	pastTime := time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339)
	s := models.Settings{
		DisplayType:       models.DisplayWaveshare75V2,
		RefreshInterval:   1, // 1 second
		LastClientRefresh: pastTime,
	}
	data, _ := json.MarshalIndent(&s, "", "  ")
	os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644)

	status, err := svc.GetRefreshStatus()
	if err != nil {
		t.Fatalf("GetRefreshStatus: %v", err)
	}
	if !status.ShouldRefresh {
		t.Error("expected should_refresh=true when interval elapsed")
	}
}

func TestSettingsService_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare75V2)

	settings, _ := svc.GetSettings()
	settings.RefreshInterval = 7200
	svc.SaveSettings(settings)

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Errorf("settings.json should exist: %v", err)
	}

	// Create new service instance, should load same data
	svc2 := NewSettingsService(dir, models.DisplayWaveshare75V2)
	loaded, _ := svc2.GetSettings()
	if loaded.RefreshInterval != 7200 {
		t.Errorf("expected 7200, got %d", loaded.RefreshInterval)
	}
}

func readSettingsFile(t *testing.T, dir string) models.Settings {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings models.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	return settings
}

// AC2: fresh install with a configured default -> that default wins.
func TestSettingsService_DefaultDisplayTypeFromEnv(t *testing.T) {
	svc := NewSettingsService(t.TempDir(), models.DisplayWaveshare75V2)

	settings, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.DisplayType != models.DisplayWaveshare75V2 {
		t.Errorf("expected display type %q, got %q", models.DisplayWaveshare75V2, settings.DisplayType)
	}
	if got := len(svc.GetDisplayConfig().Colors); got != 2 {
		t.Errorf("expected 2 colors for B/W profile, got %d", got)
	}
}

// AC3: fresh install without a configured default -> waveshare_7in3_e.
func TestSettingsService_DefaultDisplayTypeFallback(t *testing.T) {
	svc := NewSettingsService(t.TempDir(), "")

	settings, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.DisplayType != models.DisplayWaveshare73E {
		t.Errorf("expected display type %q, got %q", models.DisplayWaveshare73E, settings.DisplayType)
	}
	cfg := svc.GetDisplayConfig()
	if cfg.Driver != "epd7in3e" {
		t.Errorf("expected driver epd7in3e, got %q", cfg.Driver)
	}
	if len(cfg.Colors) != 6 {
		t.Errorf("expected 6 colors, got %d", len(cfg.Colors))
	}
}

// AC4: an existing settings.json always wins over the configured default,
// including after TriggerRefresh and RecordClientRefresh persist the file.
func TestSettingsService_ExistingSettingsWinOverDefault(t *testing.T) {
	dir := t.TempDir()
	s := models.Settings{
		DisplayType:     models.DisplayWaveshare75V2,
		RefreshInterval: defaultRefreshInterval,
	}
	data, _ := json.MarshalIndent(&s, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	svc := NewSettingsService(dir, models.DisplayWaveshare73E)

	settings, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.DisplayType != models.DisplayWaveshare75V2 {
		t.Errorf("expected persisted display type %q, got %q", models.DisplayWaveshare75V2, settings.DisplayType)
	}

	if _, err := svc.TriggerRefresh(); err != nil {
		t.Fatalf("TriggerRefresh: %v", err)
	}
	if got := readSettingsFile(t, dir).DisplayType; got != models.DisplayWaveshare75V2 {
		t.Errorf("after TriggerRefresh: expected %q, got %q", models.DisplayWaveshare75V2, got)
	}

	if err := svc.RecordClientRefresh(""); err != nil {
		t.Fatalf("RecordClientRefresh: %v", err)
	}
	if got := readSettingsFile(t, dir).DisplayType; got != models.DisplayWaveshare75V2 {
		t.Errorf("after RecordClientRefresh: expected %q, got %q", models.DisplayWaveshare75V2, got)
	}
}

// AC5: invalid default values (e.g. the driver string epd7in3e) fall back to
// waveshare_7in3_e instead of silently degrading to the B/W profile.
func TestSettingsService_InvalidDefaultDisplayTypeFallsBack(t *testing.T) {
	for _, invalid := range []string{"epd7in3e", "banana"} {
		t.Run(invalid, func(t *testing.T) {
			svc := NewSettingsService(t.TempDir(), models.DisplayType(invalid))

			settings, err := svc.GetSettings()
			if err != nil {
				t.Fatalf("GetSettings: %v", err)
			}
			if settings.DisplayType != models.DisplayWaveshare73E {
				t.Errorf("expected fallback %q, got %q", models.DisplayWaveshare73E, settings.DisplayType)
			}
			if got := svc.GetDisplayConfig().Driver; got != "epd7in3e" {
				t.Errorf("expected driver epd7in3e, got %q", got)
			}
		})
	}
}

// E1.6 AC1: missing settings.json and settings.json without the new fields
// both resolve to dither_algorithm=floyd_steinberg and calibration=default.
func TestSettingsService_DitherCalibrationDefaults(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		svc := NewSettingsService(t.TempDir(), models.DisplayWaveshare73E)

		settings, err := svc.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if settings.DitherAlgorithm != models.DitherFloydSteinberg {
			t.Errorf("expected dither_algorithm %q, got %q", models.DitherFloydSteinberg, settings.DitherAlgorithm)
		}
		if settings.Calibration != models.CalibrationDefault {
			t.Errorf("expected calibration %q, got %q", models.CalibrationDefault, settings.Calibration)
		}
	})

	t.Run("file_without_fields", func(t *testing.T) {
		dir := t.TempDir()
		legacy := `{"display_type":"waveshare_7in3_e","refresh_interval":3600}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(legacy), 0644); err != nil {
			t.Fatalf("write settings.json: %v", err)
		}
		svc := NewSettingsService(dir, models.DisplayWaveshare73E)

		settings, err := svc.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if settings.DitherAlgorithm != models.DitherFloydSteinberg {
			t.Errorf("expected dither_algorithm %q, got %q", models.DitherFloydSteinberg, settings.DitherAlgorithm)
		}
		if settings.Calibration != models.CalibrationDefault {
			t.Errorf("expected calibration %q, got %q", models.CalibrationDefault, settings.Calibration)
		}
	})

	t.Run("getters", func(t *testing.T) {
		svc := NewSettingsService(t.TempDir(), models.DisplayWaveshare73E)
		if got := svc.GetDitherAlgorithm(); got != models.DitherFloydSteinberg {
			t.Errorf("GetDitherAlgorithm: expected %q, got %q", models.DitherFloydSteinberg, got)
		}
		if got := svc.GetCalibration(); got != models.CalibrationDefault {
			t.Errorf("GetCalibration: expected %q, got %q", models.CalibrationDefault, got)
		}
	})
}

// E1.6 AC1: non-default values survive a save/load roundtrip and are
// reflected by the getters.
func TestSettingsService_DitherCalibrationRoundtrip(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare73E)

	settings, _ := svc.GetSettings()
	settings.DitherAlgorithm = models.DitherAtkinson
	settings.Calibration = models.CalibrationOff
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	fresh := NewSettingsService(dir, models.DisplayWaveshare73E)
	loaded, err := fresh.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after save: %v", err)
	}
	if loaded.DitherAlgorithm != models.DitherAtkinson {
		t.Errorf("expected dither_algorithm %q, got %q", models.DitherAtkinson, loaded.DitherAlgorithm)
	}
	if loaded.Calibration != models.CalibrationOff {
		t.Errorf("expected calibration %q, got %q", models.CalibrationOff, loaded.Calibration)
	}
	if got := fresh.GetDitherAlgorithm(); got != models.DitherAtkinson {
		t.Errorf("GetDitherAlgorithm: expected %q, got %q", models.DitherAtkinson, got)
	}
	if got := fresh.GetCalibration(); got != models.CalibrationOff {
		t.Errorf("GetCalibration: expected %q, got %q", models.CalibrationOff, got)
	}
}

// AC6: on a fresh install, TriggerRefresh and RecordClientRefresh persist the
// configured default instead of pinning the old hardcoded B/W profile.
func TestSettingsService_FreshInstallPersistsConfiguredDefault(t *testing.T) {
	t.Run("TriggerRefresh", func(t *testing.T) {
		dir := t.TempDir()
		svc := NewSettingsService(dir, models.DisplayWaveshare73E)

		if _, err := svc.TriggerRefresh(); err != nil {
			t.Fatalf("TriggerRefresh: %v", err)
		}
		if got := readSettingsFile(t, dir).DisplayType; got != models.DisplayWaveshare73E {
			t.Errorf("expected persisted %q, got %q", models.DisplayWaveshare73E, got)
		}
	})

	t.Run("RecordClientRefresh", func(t *testing.T) {
		dir := t.TempDir()
		svc := NewSettingsService(dir, models.DisplayWaveshare73E)

		if err := svc.RecordClientRefresh(""); err != nil {
			t.Fatalf("RecordClientRefresh: %v", err)
		}
		if got := readSettingsFile(t, dir).DisplayType; got != models.DisplayWaveshare73E {
			t.Errorf("expected persisted %q, got %q", models.DisplayWaveshare73E, got)
		}
	})
}
