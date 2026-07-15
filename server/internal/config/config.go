package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	Port                 string
	DataDir              string
	SecretKey            string
	DeploymentMode       string
	CORSAllowedOrigins   string
	WeatherAPIKey        string
	WeatherLocation      string
	EInkClientURL        string
	EInkDisplayType      string
	MaxConcurrentRenders int
}

func Load() *Config {
	maxRenders, err := ParseMaxConcurrentRenders(os.Getenv("EINK_MAX_CONCURRENT_RENDERS"))
	if err != nil {
		slog.Warn("invalid EINK_MAX_CONCURRENT_RENDERS, using default",
			"value", os.Getenv("EINK_MAX_CONCURRENT_RENDERS"),
			"default", DefaultMaxConcurrentRenders,
			"error", err)
	}

	return &Config{
		Port:                 getEnv("PORT", "5000"),
		DataDir:              getEnv("DATA_DIR", "./data"),
		SecretKey:            getEnv("SECRET_KEY", "dev-secret"),
		DeploymentMode:       getEnv("DEPLOYMENT_MODE", "local"),
		CORSAllowedOrigins:   getEnv("CORS_ALLOWED_ORIGINS", "*"),
		WeatherAPIKey:        getEnv("WEATHER_API_KEY", ""),
		WeatherLocation:      getEnv("WEATHER_LOCATION", ""),
		EInkClientURL:        getEnv("EINK_CLIENT_URL", ""),
		EInkDisplayType:      getEnv("EINK_DISPLAY_TYPE", ""),
		MaxConcurrentRenders: maxRenders,
	}
}

// DefaultMaxConcurrentRenders is the render semaphore capacity when
// EINK_MAX_CONCURRENT_RENDERS is unset or invalid.
const DefaultMaxConcurrentRenders = 1

// ParseMaxConcurrentRenders parses EINK_MAX_CONCURRENT_RENDERS (int >= 1).
// An empty value yields the default without error; invalid values yield the
// default alongside a non-nil error so the caller can log a warning.
func ParseMaxConcurrentRenders(v string) (int, error) {
	if v == "" {
		return DefaultMaxConcurrentRenders, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMaxConcurrentRenders, fmt.Errorf("not an integer: %q", v)
	}
	if n < 1 {
		return DefaultMaxConcurrentRenders, fmt.Errorf("must be >= 1, got %d", n)
	}
	return n, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
