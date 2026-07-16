package services

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestNoTokenOnSlogLines is the static half of AC-SEC2: no slog call in ANY of
// the four HA source files passes the token or a Bearer value as an argument.
// It names all four files explicitly (per the spec grep gate) and flags any
// slog line that mentions "token" or "bearer" (case-insensitive), so a future
// edit that logs the credential fails the build. Log MESSAGES must therefore
// avoid those words entirely (env-var names use uppercase EINK_HASS_TOKEN,
// which this check would catch, so they are phrased without it).
func TestNoTokenOnSlogLines(t *testing.T) {
	files := []string{
		"../hass/hass.go",     // config manager (persists the token)
		"hass.go",             // fetch/fill service
		"safefetch.go",        // hardened fetch helper
		"../handlers/hass.go", // admin config endpoints
	}
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", path, err)
			}
			defer f.Close()

			sc := bufio.NewScanner(f)
			line := 0
			for sc.Scan() {
				line++
				text := sc.Text()
				if !strings.Contains(text, "slog.") {
					continue
				}
				lower := strings.ToLower(text)
				if strings.Contains(lower, "token") || strings.Contains(lower, "bearer") {
					t.Errorf("%s:%d slog line references token/bearer — must never log the credential:\n\t%s",
						path, line, strings.TrimSpace(text))
				}
			}
			if err := sc.Err(); err != nil {
				t.Fatalf("scan %s: %v", path, err)
			}
		})
	}
}
