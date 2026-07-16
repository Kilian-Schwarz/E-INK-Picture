package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"e-ink-picture/server/internal/models"
)

const defaultRefreshInterval = 3600

// SettingsService manages application settings persistence.
type SettingsService struct {
	dataDir            string
	defaultDisplayType models.DisplayType
	mu                 sync.RWMutex
	// now is the clock used by GetRefreshStatus; injectable for tests.
	now func() time.Time

	// notifyMu guards notifyCh only. It is deliberately NOT s.mu: a parked
	// WaitForRefresh must hold no lock across its select, and TriggerRefresh
	// broadcasts while already holding s.mu, so the two mutexes must stay
	// independent to avoid lock-ordering hazards.
	notifyMu sync.Mutex
	// notifyCh is a closed-channel broadcast: waiters capture the current
	// reference and select on it; TriggerRefresh closes it and installs a
	// fresh channel to wake every parked waiter at once.
	notifyCh chan struct{}
}

// NewSettingsService creates a new SettingsService. defaultDisplayType is used
// whenever settings.json has no display_type value; an empty value falls back
// to models.DisplayWaveshare73E (the panel matching the client's default
// driver epd7in3e), an unknown value logs a warning and falls back likewise.
func NewSettingsService(dataDir string, defaultDisplayType models.DisplayType) *SettingsService {
	if defaultDisplayType == "" {
		defaultDisplayType = models.DisplayWaveshare73E
	} else if _, ok := models.DisplayProfiles[defaultDisplayType]; !ok {
		slog.Warn("unknown display type default, falling back",
			"value", string(defaultDisplayType),
			"fallback", string(models.DisplayWaveshare73E))
		defaultDisplayType = models.DisplayWaveshare73E
	}
	return &SettingsService{
		dataDir:            dataDir,
		defaultDisplayType: defaultDisplayType,
		now:                time.Now,
		notifyCh:           make(chan struct{}),
	}
}

// parseHHMM parses a wall-clock time in strict "HH:MM" (24h) format and
// returns it as minutes since midnight.
func parseHHMM(v string) (int, error) {
	if len(v) != 5 || v[2] != ':' || !isDigits(v[:2]) || !isDigits(v[3:]) {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", v)
	}
	h := int(v[0]-'0')*10 + int(v[1]-'0')
	m := int(v[3]-'0')*10 + int(v[4]-'0')
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", v)
	}
	return h*60 + m, nil
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ValidateSleepWindow validates a sleep_start/sleep_end pair: both empty
// disables the window; otherwise both must be valid "HH:MM" values and
// must differ (start == end would be ambiguous).
func ValidateSleepWindow(start, end string) error {
	if start == "" && end == "" {
		return nil
	}
	if start == "" || end == "" {
		return errors.New("sleep_start and sleep_end must both be set or both be empty")
	}
	s, err := parseHHMM(start)
	if err != nil {
		return fmt.Errorf("sleep_start: %w", err)
	}
	e, err := parseHHMM(end)
	if err != nil {
		return fmt.Errorf("sleep_end: %w", err)
	}
	if s == e {
		return errors.New("sleep_start and sleep_end must differ")
	}
	return nil
}

// inSleepWindow reports whether minute-of-day m lies in the half-open window
// [start, end). A window with start > end wraps across midnight
// (e.g. 23:00-06:00); start == end is treated as no window (fail open).
func inSleepWindow(m, start, end int) bool {
	switch {
	case start == end:
		return false
	case start < end:
		return m >= start && m < end
	default:
		return m >= start || m < end
	}
}

// sleepWindowActive reports whether now falls into the configured sleep
// window. Empty or invalid values fail open (window inactive).
func sleepWindowActive(start, end string, now time.Time) bool {
	if start == "" || end == "" {
		return false
	}
	s, err1 := parseHHMM(start)
	e, err2 := parseHHMM(end)
	if err1 != nil || err2 != nil {
		return false
	}
	return inSleepWindow(now.Hour()*60+now.Minute(), s, e)
}

func (s *SettingsService) filePath() string {
	return filepath.Join(s.dataDir, "settings.json")
}

// GetSettings loads settings from disk, returning defaults if the file doesn't exist.
func (s *SettingsService) GetSettings() (*models.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &models.Settings{
				DisplayType:     s.defaultDisplayType,
				RefreshInterval: defaultRefreshInterval,
				DitherAlgorithm: models.DitherFloydSteinberg,
				Calibration:     models.CalibrationDefault,
			}, nil
		}
		return nil, err
	}

	var settings models.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return &models.Settings{
			DisplayType:     s.defaultDisplayType,
			RefreshInterval: defaultRefreshInterval,
			DitherAlgorithm: models.DitherFloydSteinberg,
			Calibration:     models.CalibrationDefault,
		}, nil
	}

	if settings.DisplayType == "" {
		settings.DisplayType = s.defaultDisplayType
	}
	if settings.RefreshInterval <= 0 {
		settings.RefreshInterval = defaultRefreshInterval
	}
	if settings.RenderQuality == "" {
		settings.RenderQuality = models.RenderQualityHigh
	}
	if settings.DitherAlgorithm == "" {
		settings.DitherAlgorithm = models.DitherFloydSteinberg
	}
	if settings.Calibration == "" {
		settings.Calibration = models.CalibrationDefault
	}
	// Defensive normalization: invalid sleep window values on disk (e.g.
	// hand-edited) disable the window. Fail open towards refreshing, never
	// towards a frozen panel.
	if settings.SleepStart != "" || settings.SleepEnd != "" {
		if err := ValidateSleepWindow(settings.SleepStart, settings.SleepEnd); err != nil {
			slog.Warn("invalid sleep window in settings, treating as disabled",
				"sleep_start", settings.SleepStart,
				"sleep_end", settings.SleepEnd,
				"error", err)
			settings.SleepStart = ""
			settings.SleepEnd = ""
		}
	}

	return &settings, nil
}

// userSettingsMarkers are the raw settings.json keys that only the UI save
// path (POST /update_settings -> GetSettings -> SaveSettings) materializes.
// The heartbeat/trigger writers (RecordClientRefresh/TriggerRefresh)
// roundtrip the Settings struct whose omitempty tags keep these keys out of
// the file — their presence therefore marks a deliberate user configuration
// (specs/E2.3-setup-wizard.md, Fakt 2 / Richtung 1c).
var userSettingsMarkers = []string{
	"render_quality",
	"dither_algorithm",
	"calibration",
	"sleep_start",
	"sleep_end",
}

// HasUserSettings reports whether settings.json carries at least one
// user-set marker key. A missing file counts as fresh; a corrupt file counts
// as touched (conservative: in doubt, no setup wizard).
func (s *SettingsService) HasUserSettings() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return true, nil
	}
	for _, key := range userSettingsMarkers {
		if _, ok := raw[key]; ok {
			return true, nil
		}
	}
	return false, nil
}

// SaveSettings writes settings to disk.
func (s *SettingsService) SaveSettings(settings *models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), data, 0644)
}

// GetDisplayConfig returns the currently configured display profile.
func (s *SettingsService) GetDisplayConfig() models.DisplayConfig {
	settings, err := s.GetSettings()
	if err != nil {
		return models.GetDisplayConfig(s.defaultDisplayType)
	}
	return models.GetDisplayConfig(settings.DisplayType)
}

// GetSettingsResponse builds the full API response with resolved display config.
func (s *SettingsService) GetSettingsResponse() (*models.SettingsResponse, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	return &models.SettingsResponse{
		DisplayType:     settings.DisplayType,
		Display:         models.GetDisplayConfig(settings.DisplayType),
		RefreshInterval: settings.RefreshInterval,
		RenderQuality:   settings.RenderQuality,
		DitherAlgorithm: settings.DitherAlgorithm,
		Calibration:     settings.Calibration,
		SleepStart:      settings.SleepStart,
		SleepEnd:        settings.SleepEnd,
	}, nil
}

// GetRenderQuality returns the configured render quality setting.
func (s *SettingsService) GetRenderQuality() models.RenderQuality {
	settings, err := s.GetSettings()
	if err != nil {
		return models.RenderQualityHigh
	}
	return settings.RenderQuality
}

// GetDitherAlgorithm returns the configured dithering algorithm.
func (s *SettingsService) GetDitherAlgorithm() models.DitherAlgorithm {
	settings, err := s.GetSettings()
	if err != nil {
		return models.DitherFloydSteinberg
	}
	return settings.DitherAlgorithm
}

// GetCalibration returns the configured calibration mode.
func (s *SettingsService) GetCalibration() models.CalibrationMode {
	settings, err := s.GetSettings()
	if err != nil {
		return models.CalibrationDefault
	}
	return settings.Calibration
}

// TriggerRefresh sets the last refresh trigger timestamp.
func (s *SettingsService) TriggerRefresh() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath())
	var settings models.Settings
	if err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings.DisplayType == "" {
		settings.DisplayType = s.defaultDisplayType
	}
	if settings.RefreshInterval <= 0 {
		settings.RefreshInterval = defaultRefreshInterval
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	settings.LastRefreshTrigger = ts

	out, err := json.MarshalIndent(&settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(s.filePath(), out, 0644); err != nil {
		return "", err
	}
	// Broadcast only after the trigger is durably on disk: a woken waiter
	// re-reads settings.json via GetRefreshStatus and must see this trigger.
	s.broadcastRefresh()
	return ts, nil
}

// currentNotify returns the live notify channel. Held only for the read, never
// across a select, so a parked WaitForRefresh holds no lock.
func (s *SettingsService) currentNotify() <-chan struct{} {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	return s.notifyCh
}

// broadcastRefresh wakes every parked WaitForRefresh: closing the current
// channel releases all waiters selecting on it, and a fresh channel arms the
// next round.
func (s *SettingsService) broadcastRefresh() {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	close(s.notifyCh)
	s.notifyCh = make(chan struct{})
}

// WaitForRefresh long-polls the refresh decision: it returns immediately when a
// refresh is already due, otherwise it parks until a trigger fires, maxWait
// elapses, or ctx is done — then re-evaluates and returns. It never blocks on a
// render and holds no lock while parked. A canceled ctx is not an error: the
// caller (the HTTP handler) still answers should_refresh=false with 200.
func (s *SettingsService) WaitForRefresh(ctx context.Context, maxWait time.Duration) (*models.RefreshStatus, error) {
	// Capture the notify channel BEFORE checking status so a trigger firing
	// between the check and the park still wakes us (lost-wakeup safe).
	notify := s.currentNotify()

	status, err := s.GetRefreshStatus()
	if err != nil {
		return nil, err
	}
	if status.ShouldRefresh {
		return status, nil
	}

	select {
	case <-notify:
		// A trigger fired: re-read the freshly persisted state.
		return s.GetRefreshStatus()
	case <-time.After(maxWait):
		return s.GetRefreshStatus()
	case <-ctx.Done():
		// Client disconnect or server shutdown: report the current status
		// (normally should_refresh=false), never ctx.Err().
		return s.GetRefreshStatus()
	}
}

// RecordClientRefresh records when the client last refreshed the display.
func (s *SettingsService) RecordClientRefresh(_ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath())
	var settings models.Settings
	if err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings.DisplayType == "" {
		settings.DisplayType = s.defaultDisplayType
	}
	if settings.RefreshInterval <= 0 {
		settings.RefreshInterval = defaultRefreshInterval
	}
	settings.LastClientRefresh = time.Now().UTC().Format(time.RFC3339)

	out, err := json.MarshalIndent(&settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), out, 0644)
}

// GetRefreshStatus determines if the client should refresh.
func (s *SettingsService) GetRefreshStatus() (*models.RefreshStatus, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	now := s.now()
	shouldRefresh := false
	reason := ""

	// Check if manual trigger is newer than last client refresh. A manual
	// trigger deliberately breaks through the sleep window: an explicit
	// user action wins.
	if settings.LastRefreshTrigger != "" {
		if settings.LastClientRefresh == "" {
			shouldRefresh = true
			reason = models.RefreshReasonManual
		} else {
			triggerTime, err1 := time.Parse(time.RFC3339, settings.LastRefreshTrigger)
			clientTime, err2 := time.Parse(time.RFC3339, settings.LastClientRefresh)
			if err1 == nil && err2 == nil && triggerTime.After(clientTime) {
				shouldRefresh = true
				reason = models.RefreshReasonManual
			}
		}
	}

	// Check if refresh interval has elapsed. Interval refreshes are
	// suppressed inside the sleep window (local wall-clock time), except on
	// first start (last_client_refresh empty): a factory-new panel must
	// show content instead of staying blank all night.
	if !shouldRefresh && settings.RefreshInterval > 0 {
		if settings.LastClientRefresh == "" {
			shouldRefresh = true
			reason = models.RefreshReasonInterval
		} else if !sleepWindowActive(settings.SleepStart, settings.SleepEnd, now) {
			clientTime, err := time.Parse(time.RFC3339, settings.LastClientRefresh)
			if err == nil && now.Sub(clientTime) > time.Duration(settings.RefreshInterval)*time.Second {
				shouldRefresh = true
				reason = models.RefreshReasonInterval
			}
		}
	}

	slog.Debug("refresh_status",
		"should_refresh", shouldRefresh,
		"reason", reason,
		"refresh_interval", settings.RefreshInterval,
		"last_client_refresh", settings.LastClientRefresh,
		"last_trigger", settings.LastRefreshTrigger,
		"sleep_start", settings.SleepStart,
		"sleep_end", settings.SleepEnd,
	)

	return &models.RefreshStatus{
		ShouldRefresh:     shouldRefresh,
		Reason:            reason,
		RefreshInterval:   settings.RefreshInterval,
		LastTrigger:       settings.LastRefreshTrigger,
		LastClientRefresh: settings.LastClientRefresh,
	}, nil
}
