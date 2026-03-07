package services

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var defaultHTTPClient = http.Client{Timeout: 10 * time.Second}

// readLimitedBody reads up to limit bytes from a reader.
func readLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

// --- RSS Feed ---

type rssItem struct {
	Title       string
	Description string
	Link        string
	PubDate     string
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

// fetchRSSFeed fetches and parses an RSS feed, returning up to maxItems items.
func fetchRSSFeed(feedURL string, maxItems int) []rssItem {
	if feedURL == "" {
		return nil
	}

	resp, err := defaultHTTPClient.Get(feedURL)
	if err != nil {
		slog.Error("failed to fetch RSS feed", "url", feedURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := readLimitedBody(resp.Body, 1<<20) // 1MB limit
	if err != nil {
		return nil
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		slog.Error("failed to parse RSS feed", "url", feedURL, "error", err)
		return nil
	}

	var items []rssItem
	for i, item := range feed.Channel.Items {
		if i >= maxItems {
			break
		}
		items = append(items, rssItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			PubDate:     item.PubDate,
		})
	}
	return items
}

// --- Custom API ---

// fetchCustomAPI fetches a URL and optionally extracts a value using a simple JSON path.
func fetchCustomAPI(url string, props map[string]any) string {
	if url == "" {
		return ""
	}

	resp, err := defaultHTTPClient.Get(url)
	if err != nil {
		slog.Error("failed to fetch custom API", "url", url, "error", err)
		return "Error"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	body, err := readLimitedBody(resp.Body, 1<<20)
	if err != nil {
		return "Error"
	}

	jsonPath := GetPropString(props, "jsonPath", "")
	if jsonPath == "" {
		// Return raw text, truncated
		text := strings.TrimSpace(string(body))
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		return text
	}

	// Simple JSON path extraction (supports dot notation: "data.value")
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return string(body)
	}

	result := extractJSONPath(data, jsonPath)
	return fmt.Sprintf("%v", result)
}

// extractJSONPath extracts a value from parsed JSON using dot notation.
func extractJSONPath(data any, path string) any {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return "N/A"
			}
			current = val
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return "N/A"
			}
			current = v[idx]
		default:
			return "N/A"
		}
	}
	return current
}

// --- System Info ---

// fetchSystemInfo reads basic system metrics from /proc.
func fetchSystemInfo(props map[string]any) string {
	var lines []string
	showLabels := GetPropBool(props, "showLabels", true)

	// CPU load average
	if loadData, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(loadData))
		if len(parts) >= 3 {
			if showLabels {
				lines = append(lines, fmt.Sprintf("Load: %s %s %s", parts[0], parts[1], parts[2]))
			} else {
				lines = append(lines, fmt.Sprintf("%s %s %s", parts[0], parts[1], parts[2]))
			}
		}
	} else {
		if showLabels {
			lines = append(lines, "Load: N/A")
		} else {
			lines = append(lines, "N/A")
		}
	}

	// Memory
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		memLines := strings.Split(string(memData), "\n")
		var totalKB, availKB int64
		for _, line := range memLines {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d kB", &totalKB)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d kB", &availKB)
			}
		}
		if totalKB > 0 {
			usedMB := (totalKB - availKB) / 1024
			totalMB := totalKB / 1024
			if showLabels {
				lines = append(lines, fmt.Sprintf("RAM: %dMB / %dMB", usedMB, totalMB))
			} else {
				lines = append(lines, fmt.Sprintf("%dMB / %dMB", usedMB, totalMB))
			}
		}
	} else {
		if showLabels {
			lines = append(lines, "RAM: N/A")
		} else {
			lines = append(lines, "N/A")
		}
	}

	// CPU temperature
	if tempData, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		tempStr := strings.TrimSpace(string(tempData))
		if tempMilliC, err := strconv.ParseInt(tempStr, 10, 64); err == nil {
			tempC := float64(tempMilliC) / 1000.0
			if showLabels {
				lines = append(lines, fmt.Sprintf("Temp: %.1f°C", tempC))
			} else {
				lines = append(lines, fmt.Sprintf("%.1f°C", tempC))
			}
		}
	} else {
		if showLabels {
			lines = append(lines, "Temp: N/A")
		} else {
			lines = append(lines, "N/A")
		}
	}

	if len(lines) == 0 {
		return "No system data"
	}
	return strings.Join(lines, "\n")
}
