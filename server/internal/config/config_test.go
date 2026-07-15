package config

import "testing"

func TestParseBoolEnv(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"not-a-bool", false},
	}
	for _, c := range cases {
		if got := ParseBoolEnv("EINK_COOKIE_SECURE", c.value); got != c.want {
			t.Errorf("ParseBoolEnv(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}
