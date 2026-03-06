package services

import (
	"testing"
	"time"
)

func TestWeatherService_CacheEviction(t *testing.T) {
	svc := NewWeatherService("", "", t.TempDir())

	// Fill cache beyond max
	svc.mu.Lock()
	for i := 0; i < maxWeatherCacheEntries+10; i++ {
		key := "key_" + time.Now().Add(time.Duration(i)*time.Millisecond).Format("150405.000")
		svc.cache[key] = &weatherCacheEntry{
			data:     &WeatherData{CurrentTemp: float64(i)},
			cachedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		svc.evictOldestCache()
	}
	svc.mu.Unlock()

	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if len(svc.cache) > maxWeatherCacheEntries {
		t.Errorf("cache size %d exceeds max %d", len(svc.cache), maxWeatherCacheEntries)
	}
}

func TestWeatherCodeToDescIcon(t *testing.T) {
	desc, icon := WeatherCodeToDescIcon(0, false)
	if desc != "Clear sky" {
		t.Errorf("expected 'Clear sky', got %q", desc)
	}
	if icon != "clear_day.png" {
		t.Errorf("expected 'clear_day.png', got %q", icon)
	}

	desc, icon = WeatherCodeToDescIcon(0, true)
	if desc != "Clear sky" {
		t.Errorf("expected 'Clear sky' for night, got %q", desc)
	}
	if icon != "clear_night.png" {
		t.Errorf("expected 'clear_night.png', got %q", icon)
	}

	desc, _ = WeatherCodeToDescIcon(999, false)
	if desc != "Unknown" {
		t.Errorf("expected 'Unknown' for code 999, got %q", desc)
	}
}
