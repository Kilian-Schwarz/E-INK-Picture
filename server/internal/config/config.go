package config

import "os"

type Config struct {
	Port               string
	DataDir            string
	SecretKey          string
	DeploymentMode     string
	CORSAllowedOrigins string
	WeatherAPIKey      string
	WeatherLocation    string
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "5000"),
		DataDir:            getEnv("DATA_DIR", "./data"),
		SecretKey:          getEnv("SECRET_KEY", "dev-secret"),
		DeploymentMode:     getEnv("DEPLOYMENT_MODE", "local"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
		WeatherAPIKey:      getEnv("WEATHER_API_KEY", ""),
		WeatherLocation:    getEnv("WEATHER_LOCATION", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
