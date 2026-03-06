package widgets

import (
	"context"
	"encoding/xml"
	"image"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NewsWidget fetches and displays RSS feed items.
type NewsWidget struct {
	client *http.Client
}

func NewNewsWidget() *NewsWidget {
	return &NewsWidget{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *NewsWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

type rssItem struct {
	Title       string
	Description string
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

// GetContent fetches RSS feed and returns formatted text.
func (w *NewsWidget) GetContent(props map[string]any) string {
	feedURL := getString(props, "feedUrl", "")
	maxItems := getInt(props, "maxItems", 5)
	showDesc := getBool(props, "showDescription", false)
	title := getString(props, "title", "")

	if feedURL == "" {
		return "No feed URL"
	}

	resp, err := w.client.Get(feedURL)
	if err != nil {
		slog.Error("failed to fetch RSS feed", "url", feedURL, "error", err)
		return "No news"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "No news"
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "No news"
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		slog.Error("failed to parse RSS", "error", err)
		return "No news"
	}

	var lines []string
	if title != "" {
		lines = append(lines, title)
	}

	for i, item := range feed.Channel.Items {
		if i >= maxItems {
			break
		}
		if showDesc && item.Description != "" {
			lines = append(lines, "- "+item.Title+": "+truncate(item.Description, 80))
		} else {
			lines = append(lines, "- "+item.Title)
		}
	}

	if len(lines) == 0 || (title != "" && len(lines) == 1) {
		return "No news"
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Strip HTML tags (simple approach)
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	text := result.String()
	if len(text) > maxLen {
		return text[:maxLen] + "..."
	}
	return text
}
