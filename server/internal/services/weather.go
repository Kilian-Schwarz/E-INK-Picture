package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WeatherData holds the parsed weather response matching the Python output.
type WeatherData struct {
	CurrentTemp float64         `json:"current_temp"`
	CurrentDesc string          `json:"current_desc"`
	CurrentIcon string          `json:"current_icon"`
	CurrentCode int             `json:"current_code"`
	Daily       []DailyForecast `json:"daily"`
	Hourly      []HourlyData    `json:"hourly"`
	Sunrise     string          `json:"sunrise"`
	Sunset      string          `json:"sunset"`
}

// DailyForecast represents one day of forecast data.
type DailyForecast struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Desc    string  `json:"desc"`
	Icon    string  `json:"icon"`
	Date    string  `json:"date"`
	Weekday string  `json:"weekday"`
}

// HourlyData represents one hourly data point (every 2 hours like the Python version).
type HourlyData struct {
	Time   string  `json:"time"`
	Temp   float64 `json:"temp"`
	Desc   string  `json:"desc"`
	Icon   string  `json:"icon"`
	Precip float64 `json:"precip"`
}

// LocationResult represents a single Nominatim search result.
type LocationResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

type weatherCacheEntry struct {
	data     *WeatherData
	cachedAt time.Time
}

// openMeteoResponse mirrors the relevant parts of the open-meteo API response.
type openMeteoResponse struct {
	CurrentWeather struct {
		Temperature float64 `json:"temperature"`
		WeatherCode int     `json:"weathercode"`
	} `json:"current_weather"`
	Daily struct {
		Time            []string  `json:"time"`
		WeatherCode     []int     `json:"weathercode"`
		TemperatureMax  []float64 `json:"temperature_2m_max"`
		TemperatureMin  []float64 `json:"temperature_2m_min"`
		Sunrise         []string  `json:"sunrise"`
		Sunset          []string  `json:"sunset"`
	} `json:"daily"`
	Hourly struct {
		Time          []string  `json:"time"`
		Temperature2m []float64 `json:"temperature_2m"`
		WeatherCode   []int     `json:"weathercode"`
		Precipitation []float64 `json:"precipitation"`
	} `json:"hourly"`
}

const maxWeatherCacheEntries = 50

type WeatherService struct {
	apiKey    string
	location  string
	stylesDir string
	mu        sync.RWMutex
	cache     map[string]*weatherCacheEntry
	client    *http.Client
}

func NewWeatherService(apiKey, location, dataDir string) *WeatherService {
	return &WeatherService{
		apiKey:    apiKey,
		location:  location,
		stylesDir: filepath.Join(dataDir, "weather_styles"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]*weatherCacheEntry),
	}
}

// evictOldestCache removes the oldest cache entry when cache exceeds max size.
// Must be called with write lock held.
func (s *WeatherService) evictOldestCache() {
	if len(s.cache) <= maxWeatherCacheEntries {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	for k, v := range s.cache {
		if oldestKey == "" || v.cachedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.cachedAt
		}
	}
	if oldestKey != "" {
		delete(s.cache, oldestKey)
	}
}

// Fetch uses the configured location (if set) to fetch weather data.
func (s *WeatherService) Fetch() (any, error) {
	if s.location == "" {
		return nil, nil
	}
	parts := strings.SplitN(s.location, ",", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	lat := strings.TrimSpace(parts[0])
	lon := strings.TrimSpace(parts[1])
	return s.FetchForLocation(lat, lon)
}

// FetchForLocation fetches weather data for a given latitude/longitude.
// Results are cached for 30 minutes. On error, stale cache is returned if available.
func (s *WeatherService) FetchForLocation(lat, lon string) (*WeatherData, error) {
	cacheKey := lat + "," + lon

	// Check cache
	s.mu.RLock()
	entry, ok := s.cache[cacheKey]
	s.mu.RUnlock()
	if ok && time.Since(entry.cachedAt) < 30*time.Minute {
		return entry.data, nil
	}

	// Build request URL (matches Python exactly)
	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s"+
			"&hourly=temperature_2m,weathercode,precipitation"+
			"&daily=weathercode,temperature_2m_max,temperature_2m_min,sunrise,sunset"+
			"&current_weather=true&forecast_days=4&timezone=Europe%%2FBerlin",
		url.QueryEscape(lat), url.QueryEscape(lon),
	)

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return s.returnCachedOrError(cacheKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s.returnCachedOrError(cacheKey, fmt.Errorf("open-meteo returned status %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.returnCachedOrError(cacheKey, err)
	}

	var raw openMeteoResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return s.returnCachedOrError(cacheKey, err)
	}

	// Check for current_weather presence: if both fields are zero, the key was likely absent.
	if raw.CurrentWeather.Temperature == 0 && raw.CurrentWeather.WeatherCode == 0 {
		// Verify via raw JSON check (0°C with code 0 "Clear sky" is a valid combo)
		if !bytes.Contains(body, []byte(`"current_weather"`)) {
			return s.returnCachedOrError(cacheKey, fmt.Errorf("no current_weather in response"))
		}
	}

	currentTemp := raw.CurrentWeather.Temperature
	currentCode := raw.CurrentWeather.WeatherCode
	currentDesc, currentIcon := weatherCodeToDescIcon(currentCode, false)

	// Daily forecast (4 days)
	dailyList := make([]DailyForecast, 0, 4)
	for i := 0; i < 4 && i < len(raw.Daily.Time); i++ {
		code := raw.Daily.WeatherCode[i]
		tmax := raw.Daily.TemperatureMax[i]
		tmin := raw.Daily.TemperatureMin[i]
		desc, icon := weatherCodeToDescIcon(code, false)
		dtStr := raw.Daily.Time[i]

		t, err := time.Parse("2006-01-02", dtStr)
		if err != nil {
			continue
		}
		weekday := t.Weekday().String()

		dailyList = append(dailyList, DailyForecast{
			Min:     tmin,
			Max:     tmax,
			Desc:    desc,
			Icon:    icon,
			Date:    dtStr,
			Weekday: weekday,
		})
	}

	// Hourly data (every 2 hours, matching Python: range(0, len(times), 2))
	hourlyList := make([]HourlyData, 0)
	for i := 0; i < len(raw.Hourly.Time); i += 2 {
		htime := raw.Hourly.Time[i]
		// Extract HH:MM from "2025-01-01T12:00" format
		if len(htime) >= 16 {
			htime = htime[11:16]
		}
		htemp := raw.Hourly.Temperature2m[i]
		hcode := raw.Hourly.WeatherCode[i]
		hdesc, hicon := weatherCodeToDescIcon(hcode, false)
		hprec := raw.Hourly.Precipitation[i]
		hourlyList = append(hourlyList, HourlyData{
			Time:   htime,
			Temp:   htemp,
			Desc:   hdesc,
			Icon:   hicon,
			Precip: hprec,
		})
	}

	sunrise := ""
	if len(raw.Daily.Sunrise) > 0 {
		sunrise = raw.Daily.Sunrise[0]
	}
	sunset := ""
	if len(raw.Daily.Sunset) > 0 {
		sunset = raw.Daily.Sunset[0]
	}

	data := &WeatherData{
		CurrentTemp: currentTemp,
		CurrentDesc: currentDesc,
		CurrentIcon: currentIcon,
		CurrentCode: currentCode,
		Daily:       dailyList,
		Hourly:      hourlyList,
		Sunrise:     sunrise,
		Sunset:      sunset,
	}

	// Cache the result
	s.mu.Lock()
	s.cache[cacheKey] = &weatherCacheEntry{data: data, cachedAt: time.Now()}
	s.evictOldestCache()
	s.mu.Unlock()

	return data, nil
}

// returnCachedOrError returns stale cached data if available, otherwise the error.
func (s *WeatherService) returnCachedOrError(cacheKey string, origErr error) (*WeatherData, error) {
	s.mu.RLock()
	entry, ok := s.cache[cacheKey]
	s.mu.RUnlock()
	if ok && entry.data != nil {
		return entry.data, nil
	}
	return nil, origErr
}

// ApplyStyle loads a weather style JSON template and replaces placeholders.
// Matches the Python apply_weather_style function exactly.
func (s *WeatherService) ApplyStyle(styleName string, data *WeatherData) string {
	styleFile := filepath.Join(s.stylesDir, styleName+".json")
	raw, err := os.ReadFile(styleFile)
	if err != nil {
		return "No data"
	}

	var tmpl map[string]any
	if err := json.Unmarshal(raw, &tmpl); err != nil {
		return "No data"
	}

	text, _ := tmpl["format"].(string)
	if text == "" {
		text = "No format"
	}

	text = strings.ReplaceAll(text, "{current_temp}", fmt.Sprintf("%g", data.CurrentTemp))
	text = strings.ReplaceAll(text, "{current_desc}", data.CurrentDesc)

	// Build daily forecast lines: "Weekday: min-max°C desc"
	var dfLines []string
	for _, day := range data.Daily {
		line := fmt.Sprintf("%s: %d-%d°C %s", day.Weekday, int(day.Min), int(day.Max), day.Desc)
		dfLines = append(dfLines, line)
	}
	dailyForecastText := strings.Join(dfLines, "\n")
	text = strings.ReplaceAll(text, "{daily_forecast}", dailyForecastText)

	return text
}

// ListStyles lists available weather style files (JSON) in the given directory.
// Returns basenames without the .json extension.
func (s *WeatherService) ListStyles() ([]string, error) {
	entries, err := os.ReadDir(s.stylesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var styles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".json") {
			styles = append(styles, strings.TrimSuffix(name, filepath.Ext(name)))
		}
	}
	if styles == nil {
		styles = []string{}
	}
	return styles, nil
}

// SearchLocation proxies a location query to Nominatim and returns up to 10 results.
// Matches the Python location_search function exactly.
func (s *WeatherService) SearchLocation(query string) ([]LocationResult, error) {
	if query == "" {
		return []LocationResult{}, nil
	}

	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?format=json&q=%s", url.QueryEscape(query))

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return []LocationResult{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []LocationResult{}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []LocationResult{}, nil
	}

	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return []LocationResult{}, nil
	}

	var results []LocationResult
	for i, item := range raw {
		if i >= 10 {
			break
		}
		displayName, _ := item["display_name"].(string)
		lat, _ := item["lat"].(string)
		lon, _ := item["lon"].(string)
		results = append(results, LocationResult{
			DisplayName: displayName,
			Lat:         lat,
			Lon:         lon,
		})
	}

	if results == nil {
		results = []LocationResult{}
	}
	return results, nil
}

type descIcon struct {
	desc string
	icon string
}

var weatherDayMap = map[int]descIcon{
	0:  {"Clear sky", "clear_day.png"},
	1:  {"Mainly clear", "clear_day.png"},
	2:  {"Partly cloudy", "cloudy_day.png"},
	3:  {"Overcast", "cloudy_day.png"},
	45: {"Fog", "fog_day.png"},
	48: {"Rime fog", "fog_day.png"},
	51: {"Light drizzle", "drizzle_day.png"},
	61: {"Slight rain", "rain_day.png"},
	63: {"Moderate rain", "rain_day.png"},
	65: {"Heavy rain", "rain_day.png"},
	80: {"Rain showers", "shower_day.png"},
}

var weatherNightMap = map[int]descIcon{
	0:  {"Clear sky", "clear_night.png"},
	1:  {"Mainly clear", "clear_night.png"},
	2:  {"Partly cloudy", "cloudy_night.png"},
	3:  {"Overcast", "cloudy_night.png"},
	45: {"Fog", "fog_night.png"},
	48: {"Rime fog", "fog_night.png"},
	51: {"Light drizzle", "drizzle_night.png"},
	61: {"Slight rain", "rain_night.png"},
	63: {"Moderate rain", "rain_night.png"},
	65: {"Heavy rain", "rain_night.png"},
	80: {"Rain showers", "shower_night.png"},
}

// weatherCodeToDescIcon maps WMO weather codes to description and icon filename.
func weatherCodeToDescIcon(code int, isNight bool) (string, string) {
	if isNight {
		if di, ok := weatherNightMap[code]; ok {
			return di.desc, di.icon
		}
	}
	if di, ok := weatherDayMap[code]; ok {
		return di.desc, di.icon
	}
	return "Unknown", "cloudy_day.png"
}

// WeatherCodeToDescIcon is the exported version for use by other packages.
func WeatherCodeToDescIcon(code int, isNight bool) (string, string) {
	return weatherCodeToDescIcon(code, isNight)
}
