package models

import "testing"

func TestNormalizePanelImageMode(t *testing.T) {
	cases := []struct {
		name string
		in   PanelImageMode
		want PanelImageMode
	}{
		{"empty falls back to dithered", "", PanelImageDithered},
		{"dithered stays dithered", PanelImageDithered, PanelImageDithered},
		{"original stays original", PanelImageOriginal, PanelImageOriginal},
		{"unknown falls back to dithered", "raw", PanelImageDithered},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizePanelImageMode(c.in); got != c.want {
				t.Errorf("NormalizePanelImageMode(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
