package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"e-ink-picture/server/internal/models"
)

const defaultRefreshInterval = 3600

// SettingsService manages application settings persistence.
type SettingsService struct {
	dataDir string
	mu      sync.RWMutex
}

// NewSettingsService creates a new SettingsService.
func NewSettingsService(dataDir string) *SettingsService {
	return &SettingsService{dataDir: dataDir}
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
			return &models.Settings{DisplayType: models.DisplayWaveshare75V2}, nil
		}
		return nil, err
	}

	var settings models.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return &models.Settings{DisplayType: models.DisplayWaveshare75V2}, nil
	}

	if settings.DisplayType == "" {
		settings.DisplayType = models.DisplayWaveshare75V2
	}
	if settings.RefreshInterval <= 0 {
		settings.RefreshInterval = defaultRefreshInterval
	}

	return &settings, nil
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
		return models.GetDisplayConfig(models.DisplayWaveshare75V2)
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
	}, nil
}

// TriggerRefresh sets the last refresh trigger timestamp.
func (s *SettingsService) TriggerRefresh() (string, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	settings.LastRefreshTrigger = ts
	if err := s.SaveSettings(settings); err != nil {
		return "", err
	}
	return ts, nil
}

// RecordClientRefresh records when the client last refreshed the display.
func (s *SettingsService) RecordClientRefresh(ts string) error {
	settings, err := s.GetSettings()
	if err != nil {
		return err
	}
	settings.LastClientRefresh = ts
	return s.SaveSettings(settings)
}

// GetRefreshStatus determines if the client should refresh.
func (s *SettingsService) GetRefreshStatus() (*models.RefreshStatus, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	shouldRefresh := false

	// Check if manual trigger is newer than last client refresh
	if settings.LastRefreshTrigger != "" {
		if settings.LastClientRefresh == "" {
			shouldRefresh = true
		} else {
			triggerTime, err1 := time.Parse(time.RFC3339, settings.LastRefreshTrigger)
			clientTime, err2 := time.Parse(time.RFC3339, settings.LastClientRefresh)
			if err1 == nil && err2 == nil && triggerTime.After(clientTime) {
				shouldRefresh = true
			}
		}
	}

	// Check if refresh interval has elapsed
	if !shouldRefresh && settings.RefreshInterval > 0 {
		if settings.LastClientRefresh == "" {
			shouldRefresh = true
		} else {
			clientTime, err := time.Parse(time.RFC3339, settings.LastClientRefresh)
			if err == nil && time.Since(clientTime) > time.Duration(settings.RefreshInterval)*time.Second {
				shouldRefresh = true
			}
		}
	}

	return &models.RefreshStatus{
		ShouldRefresh:     shouldRefresh,
		RefreshInterval:   settings.RefreshInterval,
		LastTrigger:       settings.LastRefreshTrigger,
		LastClientRefresh: settings.LastClientRefresh,
	}, nil
}
