package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DataDir            string
	DeploymentMode     string
	CORSAllowedOrigins string
	WeatherAPIKey      string
	WeatherLocation    string
	EInkClientURL      string
	EInkDisplayType    string
	// AdminPassword (EINK_ADMIN_PASSWORD) bootstraps the admin password on
	// startup when data/auth.json does not exist yet; it is ignored (with a
	// warning) once a password is set.
	AdminPassword string
	// ClientToken (EINK_CLIENT_TOKEN) authenticates the headless e-ink
	// client via the X-Client-Token header on its four endpoints.
	ClientToken string
	// CookieSecure (EINK_COOKIE_SECURE) forces the Secure attribute on the
	// session cookie for operation behind a TLS-terminating proxy.
	CookieSecure bool
	// HassURL (EINK_HASS_URL) and HassToken (EINK_HASS_TOKEN) bootstrap the
	// Home-Assistant connection on startup when data/hass.json does not exist
	// yet; they are ignored (with a warning) once a config is present. The
	// token value is never logged.
	HassURL              string
	HassToken            string
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
		DeploymentMode:       getEnv("DEPLOYMENT_MODE", "local"),
		CORSAllowedOrigins:   getEnv("CORS_ALLOWED_ORIGINS", ""),
		WeatherAPIKey:        getEnv("WEATHER_API_KEY", ""),
		WeatherLocation:      getEnv("WEATHER_LOCATION", ""),
		EInkClientURL:        getEnv("EINK_CLIENT_URL", ""),
		EInkDisplayType:      getEnv("EINK_DISPLAY_TYPE", ""),
		AdminPassword:        os.Getenv("EINK_ADMIN_PASSWORD"),
		ClientToken:          os.Getenv("EINK_CLIENT_TOKEN"),
		CookieSecure:         ParseBoolEnv("EINK_COOKIE_SECURE", os.Getenv("EINK_COOKIE_SECURE")),
		HassURL:              os.Getenv("EINK_HASS_URL"),
		HassToken:            os.Getenv("EINK_HASS_TOKEN"),
		MaxConcurrentRenders: maxRenders,
	}
}

// ParseBoolEnv parses a boolean env value (strconv.ParseBool syntax).
// Empty means false; invalid values log a warning and yield false.
func ParseBoolEnv(name, value string) bool {
	if value == "" {
		return false
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("invalid boolean env value, using false", "name", name, "value", value)
		return false
	}
	return b
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
