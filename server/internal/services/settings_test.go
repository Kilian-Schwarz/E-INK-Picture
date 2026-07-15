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

// --- E5.2 Phase 1: sleep window (panel care) ---

// writeSettingsFile persists a settings.json for sleep window tests.
func writeSettingsFile(t *testing.T, dir string, s models.Settings) {
	t.Helper()
	if s.DisplayType == "" {
		s.DisplayType = models.DisplayWaveshare73E
	}
	data, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// at returns a fixed test date carrying the given wall-clock time.
func at(hour, min int) time.Time {
	return time.Date(2026, 3, 4, hour, min, 0, 0, time.UTC)
}

// AC2: strict HH:MM parsing (pure function).
func TestParseHHMM(t *testing.T) {
	valid := map[string]int{
		"00:00": 0,
		"06:00": 360,
		"12:34": 754,
		"23:59": 1439,
	}
	for in, want := range valid {
		got, err := parseHHMM(in)
		if err != nil {
			t.Errorf("parseHHMM(%q): unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseHHMM(%q) = %d, want %d", in, got, want)
		}
	}

	invalid := []string{"", "23:0", "24:00", "aa:bb", "23:60", "1:00", "23-00", "+1:30", " 3:00", "23:5 "}
	for _, in := range invalid {
		if _, err := parseHHMM(in); err == nil {
			t.Errorf("parseHHMM(%q): expected error, got nil", in)
		}
	}
}

// AC2: pure window arithmetic — half-open [start, end), wrap at midnight.
func TestInSleepWindow(t *testing.T) {
	tests := []struct {
		name    string
		m, s, e int
		want    bool
	}{
		{"non-wrap before start", 719, 720, 840, false},
		{"non-wrap at start", 720, 720, 840, true},
		{"non-wrap inside", 800, 720, 840, true},
		{"non-wrap last minute", 839, 720, 840, true},
		{"non-wrap at end (half-open)", 840, 720, 840, false},
		{"wrap before start", 1379, 1380, 360, false},
		{"wrap at start", 1380, 1380, 360, true},
		{"wrap midnight", 0, 1380, 360, true},
		{"wrap early morning", 180, 1380, 360, true},
		{"wrap last minute", 359, 1380, 360, true},
		{"wrap at end (half-open)", 360, 1380, 360, false},
		{"degenerate start==end fails open", 100, 100, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inSleepWindow(tt.m, tt.s, tt.e); got != tt.want {
				t.Errorf("inSleepWindow(%d, %d, %d) = %v, want %v", tt.m, tt.s, tt.e, got, tt.want)
			}
		})
	}
}

// AC1/AC2: pair validation — both-or-none, strict format, start != end.
func TestValidateSleepWindow(t *testing.T) {
	valid := [][2]string{
		{"", ""},
		{"23:00", "06:00"},
		{"12:00", "14:00"},
		{"00:00", "23:59"},
	}
	for _, v := range valid {
		if err := ValidateSleepWindow(v[0], v[1]); err != nil {
			t.Errorf("ValidateSleepWindow(%q, %q): unexpected error: %v", v[0], v[1], err)
		}
	}

	invalid := [][2]string{
		{"23:00", ""},
		{"", "06:00"},
		{"23:0", "06:00"},
		{"24:00", "06:00"},
		{"aa:bb", "06:00"},
		{"23:00", "aa:bb"},
		{"12:00", "12:00"},
	}
	for _, v := range invalid {
		if err := ValidateSleepWindow(v[0], v[1]); err == nil {
			t.Errorf("ValidateSleepWindow(%q, %q): expected error, got nil", v[0], v[1])
		}
	}
}

// AC2: interval refreshes are suppressed inside the sleep window, evaluated
// against the injected clock with half-open [start, end) semantics.
func TestSettingsService_SleepWindowSuppressesInterval(t *testing.T) {
	tests := []struct {
		name        string
		start, end  string
		clock       time.Time
		wantRefresh bool
	}{
		{"wrap 22:59 before window", "23:00", "06:00", at(22, 59), true},
		{"wrap 23:00 window starts", "23:00", "06:00", at(23, 0), false},
		{"wrap 03:00 inside", "23:00", "06:00", at(3, 0), false},
		{"wrap 05:59 last minute", "23:00", "06:00", at(5, 59), false},
		{"wrap 06:00 window ends (half-open)", "23:00", "06:00", at(6, 0), true},
		{"day 11:59 before window", "12:00", "14:00", at(11, 59), true},
		{"day 12:00 window starts", "12:00", "14:00", at(12, 0), false},
		{"day 13:59 last minute", "12:00", "14:00", at(13, 59), false},
		{"day 14:00 window ends (half-open)", "12:00", "14:00", at(14, 0), true},
		{"window disabled", "", "", at(3, 0), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSettingsFile(t, dir, models.Settings{
				RefreshInterval:   3600,
				LastClientRefresh: "2026-03-01T00:00:00Z", // interval long elapsed
				SleepStart:        tt.start,
				SleepEnd:          tt.end,
			})
			svc := NewSettingsService(dir, models.DisplayWaveshare73E)
			svc.now = func() time.Time { return tt.clock }

			status, err := svc.GetRefreshStatus()
			if err != nil {
				t.Fatalf("GetRefreshStatus: %v", err)
			}
			if status.ShouldRefresh != tt.wantRefresh {
				t.Errorf("should_refresh = %v, want %v", status.ShouldRefresh, tt.wantRefresh)
			}
			wantReason := ""
			if tt.wantRefresh {
				wantReason = models.RefreshReasonInterval
			}
			if status.Reason != wantReason {
				t.Errorf("reason = %q, want %q", status.Reason, wantReason)
			}
		})
	}
}

// AC2 / decision 3: first start (last_client_refresh empty) refreshes even
// inside the sleep window — a factory-new panel must show content.
func TestSettingsService_SleepWindowFirstStartException(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, models.Settings{
		RefreshInterval: 3600,
		SleepStart:      "23:00",
		SleepEnd:        "06:00",
	})
	svc := NewSettingsService(dir, models.DisplayWaveshare73E)
	svc.now = func() time.Time { return at(3, 0) }

	status, err := svc.GetRefreshStatus()
	if err != nil {
		t.Fatalf("GetRefreshStatus: %v", err)
	}
	if !status.ShouldRefresh {
		t.Error("expected should_refresh=true on first start inside the sleep window")
	}
	if status.Reason != models.RefreshReasonInterval {
		t.Errorf("reason = %q, want %q", status.Reason, models.RefreshReasonInterval)
	}
}

// AC2 / decision 4: invalid sleep window values on disk normalize to
// disabled (fail open towards refreshing, never towards a frozen panel).
func TestSettingsService_SleepWindowInvalidOnDiskFailsOpen(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
	}{
		{"garbage values", "25:99", "06:00"},
		{"only start set", "23:00", ""},
		{"only end set", "", "06:00"},
		{"start equals end", "03:00", "03:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSettingsFile(t, dir, models.Settings{
				RefreshInterval:   3600,
				LastClientRefresh: "2026-03-01T00:00:00Z",
				SleepStart:        tt.start,
				SleepEnd:          tt.end,
			})
			svc := NewSettingsService(dir, models.DisplayWaveshare73E)
			svc.now = func() time.Time { return at(3, 0) } // inside the would-be window

			settings, err := svc.GetSettings()
			if err != nil {
				t.Fatalf("GetSettings: %v", err)
			}
			if settings.SleepStart != "" || settings.SleepEnd != "" {
				t.Errorf("expected window normalized off, got %q/%q", settings.SleepStart, settings.SleepEnd)
			}

			status, err := svc.GetRefreshStatus()
			if err != nil {
				t.Fatalf("GetRefreshStatus: %v", err)
			}
			if !status.ShouldRefresh {
				t.Error("expected should_refresh=true (fail open) with invalid window values")
			}
		})
	}
}

// AC3: a manual trigger breaks through an active sleep window.
func TestSettingsService_ManualTriggerBreaksSleepWindow(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, models.Settings{
		RefreshInterval:    3600,
		LastClientRefresh:  "2026-03-04T01:00:00Z",
		LastRefreshTrigger: "2026-03-04T02:00:00Z", // newer than last client refresh
		SleepStart:         "23:00",
		SleepEnd:           "06:00",
	})
	svc := NewSettingsService(dir, models.DisplayWaveshare73E)
	svc.now = func() time.Time { return at(3, 0) } // inside the window

	status, err := svc.GetRefreshStatus()
	if err != nil {
		t.Fatalf("GetRefreshStatus: %v", err)
	}
	if !status.ShouldRefresh {
		t.Error("expected should_refresh=true: manual trigger must break the sleep window")
	}
	if status.Reason != models.RefreshReasonManual {
		t.Errorf("reason = %q, want %q", status.Reason, models.RefreshReasonManual)
	}
}

// AC4: reason is "interval" for elapsed intervals, "manual" for triggers,
// "manual" when both apply (trigger checked first), empty when no refresh.
func TestSettingsService_RefreshReason(t *testing.T) {
	tests := []struct {
		name        string
		settings    models.Settings
		wantRefresh bool
		wantReason  string
	}{
		{
			name: "interval elapsed",
			settings: models.Settings{
				RefreshInterval:   3600,
				LastClientRefresh: "2026-03-01T00:00:00Z",
			},
			wantRefresh: true,
			wantReason:  models.RefreshReasonInterval,
		},
		{
			name: "manual trigger",
			settings: models.Settings{
				RefreshInterval:    3600,
				LastClientRefresh:  "2026-03-04T02:30:00Z", // interval not elapsed at 03:00
				LastRefreshTrigger: "2026-03-04T02:45:00Z",
			},
			wantRefresh: true,
			wantReason:  models.RefreshReasonManual,
		},
		{
			name: "both apply -> manual wins",
			settings: models.Settings{
				RefreshInterval:    3600,
				LastClientRefresh:  "2026-03-01T00:00:00Z",
				LastRefreshTrigger: "2026-03-04T02:00:00Z",
			},
			wantRefresh: true,
			wantReason:  models.RefreshReasonManual,
		},
		{
			name: "no refresh -> no reason",
			settings: models.Settings{
				RefreshInterval:   3600,
				LastClientRefresh: "2026-03-04T02:59:00Z",
			},
			wantRefresh: false,
			wantReason:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSettingsFile(t, dir, tt.settings)
			svc := NewSettingsService(dir, models.DisplayWaveshare73E)
			svc.now = func() time.Time { return at(3, 0) }

			status, err := svc.GetRefreshStatus()
			if err != nil {
				t.Fatalf("GetRefreshStatus: %v", err)
			}
			if status.ShouldRefresh != tt.wantRefresh {
				t.Errorf("should_refresh = %v, want %v", status.ShouldRefresh, tt.wantRefresh)
			}
			if status.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", status.Reason, tt.wantReason)
			}
		})
	}
}

// AC1: a settings.json without the sleep fields loads with the window off.
func TestSettingsService_SleepWindowLegacyFileLoads(t *testing.T) {
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
	if settings.SleepStart != "" || settings.SleepEnd != "" {
		t.Errorf("expected window off, got %q/%q", settings.SleepStart, settings.SleepEnd)
	}

	resp, err := svc.GetSettingsResponse()
	if err != nil {
		t.Fatalf("GetSettingsResponse: %v", err)
	}
	if resp.SleepStart != "" || resp.SleepEnd != "" {
		t.Errorf("expected empty sleep fields in response, got %q/%q", resp.SleepStart, resp.SleepEnd)
	}
}

// AC1: sleep window values survive a save/load roundtrip.
func TestSettingsService_SleepWindowRoundtrip(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir, models.DisplayWaveshare73E)

	settings, _ := svc.GetSettings()
	settings.SleepStart = "23:00"
	settings.SleepEnd = "06:00"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	fresh := NewSettingsService(dir, models.DisplayWaveshare73E)
	loaded, err := fresh.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after save: %v", err)
	}
	if loaded.SleepStart != "23:00" || loaded.SleepEnd != "06:00" {
		t.Errorf("expected 23:00/06:00, got %q/%q", loaded.SleepStart, loaded.SleepEnd)
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
