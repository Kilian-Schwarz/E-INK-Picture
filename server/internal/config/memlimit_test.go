package config

import "testing"

func TestParseMemLimit(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"mib_suffix", "64MiB", 64 << 20, false},
		{"mib_suffix_48", "48MiB", 48 << 20, false},
		{"mib_suffix_case_insensitive", "32mib", 32 << 20, false},
		{"bare_bytes", "67108864", 67108864, false},
		{"off", "off", 0, false},
		{"off_uppercase", "OFF", 0, false},
		{"zero", "0", 0, false},
		{"zero_mib", "0MiB", 0, false},
		{"whitespace_trimmed", " 64MiB ", 64 << 20, false},
		{"empty", "", 0, true},
		{"negative", "-1", 0, true},
		{"negative_mib", "-5MiB", 0, true},
		{"garbage", "banana", 0, true},
		{"wrong_suffix", "64MB", 0, true},
		{"float", "1.5MiB", 0, true},
		{"overflow", "99999999999999MiB", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMemLimit(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseMemLimit(%q) = %d, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMemLimit(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseMemLimit(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestResolveMemLimit covers the AC2 precedence matrix:
// (a) no env vars -> default 64 MiB applied,
// (b) EINK_GOMEMLIMIT overrides (also over native GOMEMLIMIT),
// (c) EINK_GOMEMLIMIT=off/0 -> no limit applied,
// (d) native GOMEMLIMIT set, EINK_GOMEMLIMIT empty -> runtime untouched.
func TestResolveMemLimit(t *testing.T) {
	cases := []struct {
		name       string
		einkVal    string
		nativeVal  string
		wantApply  bool
		wantBytes  int64
		wantSource MemLimitSource
		wantErr    bool
	}{
		{"a_default_no_env", "", "", true, DefaultMemLimitBytes, MemLimitSourceDefault, false},
		{"b_eink_overrides", "48MiB", "", true, 48 << 20, MemLimitSourceEinkEnv, false},
		{"b_eink_wins_over_native", "48MiB", "128MiB", true, 48 << 20, MemLimitSourceEinkEnv, false},
		{"b_eink_bare_bytes", "33554432", "", true, 32 << 20, MemLimitSourceEinkEnv, false},
		{"c_eink_off", "off", "", false, 0, MemLimitSourceEinkEnv, false},
		{"c_eink_zero", "0", "", false, 0, MemLimitSourceEinkEnv, false},
		{"c_eink_off_wins_over_native", "off", "128MiB", false, 0, MemLimitSourceEinkEnv, false},
		{"d_native_wins_when_eink_empty", "", "128MiB", false, 0, MemLimitSourceNativeEnv, false},
		{"invalid_eink_falls_back_to_default", "banana", "", true, DefaultMemLimitBytes, MemLimitSourceDefault, true},
		{"invalid_eink_with_native_still_default", "banana", "128MiB", true, DefaultMemLimitBytes, MemLimitSourceDefault, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMemLimit(tc.einkVal, tc.nativeVal)
			if tc.wantErr && err == nil {
				t.Error("expected a parse error for the warning log, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got.Apply != tc.wantApply {
				t.Errorf("Apply = %v, want %v", got.Apply, tc.wantApply)
			}
			if got.Apply && got.Bytes != tc.wantBytes {
				t.Errorf("Bytes = %d, want %d", got.Bytes, tc.wantBytes)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

func TestParseMaxConcurrentRenders(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"empty_default", "", 1, false},
		{"one", "1", 1, false},
		{"four", "4", 4, false},
		{"zero_invalid", "0", 1, true},
		{"negative_invalid", "-2", 1, true},
		{"garbage_invalid", "two", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMaxConcurrentRenders(tc.in)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseMaxConcurrentRenders(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
