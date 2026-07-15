package config

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultMemLimitBytes is the GOMEMLIMIT soft limit applied when neither
// EINK_GOMEMLIMIT nor the native GOMEMLIMIT env var is set. Derivation (see
// specs/E5.6-render-memory.md): idle baseline ~5 MB + high-quality render
// live peak ~32-35 MB ~= 40 MB live maximum with the render semaphore at 1;
// 64 MiB leaves ~1.6x headroom so the GC does not thrash during renders.
const DefaultMemLimitBytes = 64 << 20

// MemLimitSource identifies which configuration source decided the memory limit.
type MemLimitSource string

const (
	MemLimitSourceEinkEnv   MemLimitSource = "EINK_GOMEMLIMIT"
	MemLimitSourceNativeEnv MemLimitSource = "GOMEMLIMIT"
	MemLimitSourceDefault   MemLimitSource = "default"
)

// MemLimitDecision tells main whether to call debug.SetMemoryLimit and with
// which value. Apply=false means: leave the runtime untouched (either the
// native GOMEMLIMIT env var is in charge, or the limit is explicitly off).
type MemLimitDecision struct {
	Apply  bool
	Bytes  int64
	Source MemLimitSource
}

// ParseMemLimit parses an EINK_GOMEMLIMIT value: a positive integer with an
// optional MiB suffix (e.g. "64MiB"; a bare number is bytes). The special
// values "off" and "0" mean "no limit" and are returned as 0 with a nil
// error. Pure function, unit-tested.
func ParseMemLimit(v string) (int64, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	if strings.EqualFold(s, "off") {
		return 0, nil
	}

	mult := int64(1)
	num := s
	if len(s) > 3 && strings.EqualFold(s[len(s)-3:], "mib") {
		mult = 1 << 20
		num = strings.TrimSpace(s[:len(s)-3])
	}

	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a valid number: %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("must not be negative: %q", v)
	}
	if n == 0 {
		return 0, nil
	}
	if n > (1<<63-1)/mult {
		return 0, fmt.Errorf("value overflows: %q", v)
	}
	return n * mult, nil
}

// ResolveMemLimit implements the GOMEMLIMIT precedence (pure function,
// unit-tested; see specs/E5.6-render-memory.md AC2):
//
//  1. EINK_GOMEMLIMIT set   -> apply the parsed value ("off"/"0" = no limit;
//     invalid -> apply the default and return the parse error for logging).
//  2. native GOMEMLIMIT set -> do NOT touch the runtime: it honors the env
//     var itself and debug.SetMemoryLimit would override it.
//  3. otherwise             -> apply DefaultMemLimitBytes (64 MiB).
func ResolveMemLimit(einkVal, nativeVal string) (MemLimitDecision, error) {
	if einkVal != "" {
		limit, err := ParseMemLimit(einkVal)
		if err != nil {
			return MemLimitDecision{Apply: true, Bytes: DefaultMemLimitBytes, Source: MemLimitSourceDefault}, err
		}
		if limit == 0 {
			return MemLimitDecision{Apply: false, Source: MemLimitSourceEinkEnv}, nil
		}
		return MemLimitDecision{Apply: true, Bytes: limit, Source: MemLimitSourceEinkEnv}, nil
	}
	if nativeVal != "" {
		return MemLimitDecision{Apply: false, Source: MemLimitSourceNativeEnv}, nil
	}
	return MemLimitDecision{Apply: true, Bytes: DefaultMemLimitBytes, Source: MemLimitSourceDefault}, nil
}
