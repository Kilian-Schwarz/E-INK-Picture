package widgets

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CustomWidget fetches data from a custom HTTP API and extracts values via JSONPath.
type CustomWidget struct {
	client *http.Client
}

func NewCustomWidget() *CustomWidget {
	return &CustomWidget{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *CustomWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent fetches a URL and optionally extracts a JSON value.
func (w *CustomWidget) GetContent(props map[string]any) string {
	url := getString(props, "url", "")
	if url == "" {
		return "No URL configured"
	}

	resp, err := w.client.Get(url)
	if err != nil {
		slog.Error("failed to fetch custom API", "url", url, "error", err)
		return "Error"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "Error"
	}

	jsonPath := getString(props, "jsonPath", "")
	prefix := getString(props, "prefix", "")
	suffix := getString(props, "suffix", "")

	if jsonPath == "" {
		text := strings.TrimSpace(string(body))
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		return prefix + text + suffix
	}

	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return prefix + string(body) + suffix
	}

	result := extractJSONPath(data, jsonPath)
	return prefix + fmt.Sprintf("%v", result) + suffix
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
