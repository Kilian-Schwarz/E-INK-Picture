package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSettingsService_DefaultRefreshInterval(t *testing.T) {
	dir := t.TempDir()
	svc := NewSettingsService(dir)

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
	svc := NewSettingsService(dir)

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
	svc := NewSettingsService(dir)

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
	svc := NewSettingsService(dir)

	// Record a client refresh in the past
	pastTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	if err := svc.RecordClientRefresh(pastTime); err != nil {
		t.Fatalf("RecordClientRefresh: %v", err)
	}

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
	svc := NewSettingsService(dir)

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
	svc := NewSettingsService(dir)

	// Set very short interval
	settings, _ := svc.GetSettings()
	settings.RefreshInterval = 1 // 1 second
	svc.SaveSettings(settings)

	// Record a past client refresh
	pastTime := time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339)
	svc.RecordClientRefresh(pastTime)

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
	svc := NewSettingsService(dir)

	settings, _ := svc.GetSettings()
	settings.RefreshInterval = 7200
	svc.SaveSettings(settings)

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Errorf("settings.json should exist: %v", err)
	}

	// Create new service instance, should load same data
	svc2 := NewSettingsService(dir)
	loaded, _ := svc2.GetSettings()
	if loaded.RefreshInterval != 7200 {
		t.Errorf("expected 7200, got %d", loaded.RefreshInterval)
	}
}
