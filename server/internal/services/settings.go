package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"e-ink-picture/server/internal/models"
)

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
		DisplayType: settings.DisplayType,
		Display:     models.GetDisplayConfig(settings.DisplayType),
	}, nil
}
