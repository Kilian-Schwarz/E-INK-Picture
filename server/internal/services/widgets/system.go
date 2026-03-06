package widgets

import (
	"context"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
)

// SystemWidget displays system information (CPU, memory, temperature).
type SystemWidget struct{}

func NewSystemWidget() *SystemWidget {
	return &SystemWidget{}
}

func (w *SystemWidget) Render(_ context.Context, _ map[string]any, _ image.Rectangle, _ *image.RGBA) error {
	return nil
}

// GetContent reads system metrics from /proc and /sys.
func (w *SystemWidget) GetContent(props map[string]any) string {
	showLabels := getBool(props, "showLabels", true)
	var lines []string

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
	}

	if len(lines) == 0 {
		return "No system data"
	}
	return strings.Join(lines, "\n")
}
