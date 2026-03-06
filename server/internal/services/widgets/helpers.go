package widgets

import "strconv"

// getString extracts a string property with fallback.
func getString(props map[string]any, key, fallback string) string {
	if v, ok := props[key]; ok {
		switch s := v.(type) {
		case string:
			return s
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64)
		}
	}
	return fallback
}

// getInt extracts an int property with fallback.
func getInt(props map[string]any, key string, fallback int) int {
	if v, ok := props[key]; ok {
		switch f := v.(type) {
		case float64:
			return int(f)
		case string:
			if val, err := strconv.Atoi(f); err == nil {
				return val
			}
		}
	}
	return fallback
}

// getBool extracts a bool property with fallback.
func getBool(props map[string]any, key string, fallback bool) bool {
	if v, ok := props[key]; ok {
		switch b := v.(type) {
		case bool:
			return b
		case string:
			return b == "true"
		}
	}
	return fallback
}
