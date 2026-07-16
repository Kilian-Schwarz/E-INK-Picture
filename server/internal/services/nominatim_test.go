package services

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// nominatimRecorder is a recording RoundTripper for SearchLocation tests: it
// counts outgoing calls and captures the last request URL and User-Agent, then
// returns a fixed status/body (or a transport error). Distinct from
// safefetch_test.go's recordingTransport to avoid a name clash in-package.
type nominatimRecorder struct {
	mu      sync.Mutex
	calls   int
	lastURL string
	lastUA  string
	status  int
	body    string
	err     error
}

func (r *nominatimRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.calls++
	r.lastURL = req.URL.String()
	r.lastUA = req.Header.Get("User-Agent")
	status, body, err := r.status, r.body, r.err
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func (r *nominatimRecorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// canonicalLocationJSON is a Nominatim /search?addressdetails=1 response with
// three distinct look-alike places exercising the city/town/village and
// state/county priority orders, in importance order.
const canonicalLocationJSON = `[
  {
    "lat": "52.3744779", "lon": "9.7385532",
    "type": "administrative", "addresstype": "city",
    "name": "Hannover",
    "display_name": "Hannover, Region Hannover, Niedersachsen, Deutschland",
    "address": {"city": "Hannover", "state": "Niedersachsen", "country": "Deutschland", "country_code": "de"}
  },
  {
    "lat": "39.8006", "lon": "-76.9830",
    "type": "town", "addresstype": "town",
    "name": "Hanover",
    "display_name": "Hanover, York County, Pennsylvania, Vereinigte Staaten von Amerika",
    "address": {"town": "Hanover", "county": "York County", "state": "Pennsylvania", "country": "Vereinigte Staaten von Amerika", "country_code": "us"}
  },
  {
    "lat": "51.2500", "lon": "9.6500",
    "type": "administrative", "addresstype": "village",
    "name": "Beispieldorf",
    "display_name": "Beispieldorf, Landkreis Göttingen, Deutschland",
    "address": {"village": "Beispieldorf", "county": "Landkreis Göttingen", "country": "Deutschland"}
  }
]`

// postcodeLocationJSON is a Nominatim response for a German postcode query.
const postcodeLocationJSON = `[
  {
    "lat": "52.3701", "lon": "9.7332",
    "type": "postcode", "addresstype": "postcode",
    "name": "30159",
    "display_name": "30159, Hannover, Region Hannover, Niedersachsen, Deutschland",
    "address": {"postcode": "30159", "city": "Hannover", "state": "Niedersachsen", "country": "Deutschland"}
  }
]`

func newLocationTestService(t *testing.T, rec *nominatimRecorder) *WeatherService {
	t.Helper()
	svc := NewWeatherService("", "", t.TempDir())
	svc.client = &http.Client{Transport: rec}
	return svc
}

// TestNormalizeLocationQuery proves the cache key derivation: trim, lower-case
// and collapse internal whitespace so trivially different queries share a key.
func TestNormalizeLocationQuery(t *testing.T) {
	cases := map[string]string{
		"Hannover":         "hannover",
		"  Hannover  ":     "hannover",
		"HANNOVER":         "hannover",
		"New   York":       "new york",
		"\tBad Nauheim\n":  "bad nauheim",
		"Bad  \t Nauheim ": "bad nauheim",
		"":                 "",
		"   ":              "",
	}
	for in, want := range cases {
		if got := normalizeLocationQuery(in); got != want {
			t.Errorf("normalizeLocationQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMinIntervalLimiterSpacing proves the limiter enforces a >=interval gap
// with a fake clock and never sleeps: two allows in the same window return
// (true, false); advancing past the interval allows again.
func TestMinIntervalLimiterSpacing(t *testing.T) {
	l := newMinIntervalLimiter(time.Second)
	clock := newTestClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	l.setNow(clock.Now)

	if !l.allow() {
		t.Fatal("first request must be allowed")
	}
	if l.allow() {
		t.Error("second request in the same 1s window must be denied")
	}
	clock.Advance(999 * time.Millisecond)
	if l.allow() {
		t.Error("request just before the interval elapses must still be denied")
	}
	clock.Advance(time.Millisecond)
	if !l.allow() {
		t.Error("request at exactly the interval boundary must be allowed")
	}
	if l.allow() {
		t.Error("the boundary allow reserves a new slot; the next must be denied")
	}
}

// TestMinIntervalLimiterReset proves reset clears the reserved slot and
// restores the real clock.
func TestMinIntervalLimiterReset(t *testing.T) {
	l := newMinIntervalLimiter(time.Hour)
	clock := newTestClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	l.setNow(clock.Now)
	if !l.allow() {
		t.Fatal("first request must be allowed")
	}
	if l.allow() {
		t.Fatal("precondition: second request must be denied")
	}
	l.reset()
	if !l.allow() {
		t.Error("after reset the slot must be free again")
	}
}

// TestLocationSearchCacheTTL proves get returns a fresh entry and drops it once
// the TTL elapses, using an injected clock (no sleeps).
func TestLocationSearchCacheTTL(t *testing.T) {
	c := newLocationSearchCache(time.Hour, 8)
	clock := newTestClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)

	want := []LocationResult{{DisplayName: "Hannover", Lat: "52.37", Lon: "9.73"}}
	c.put("hannover", want)

	if got, ok := c.get("hannover"); !ok || len(got) != 1 || got[0].DisplayName != "Hannover" {
		t.Fatalf("fresh get = (%v, %v), want the stored entry", got, ok)
	}
	clock.Advance(time.Hour - time.Second)
	if _, ok := c.get("hannover"); !ok {
		t.Error("entry must still be served just before TTL expiry")
	}
	clock.Advance(2 * time.Second)
	if _, ok := c.get("hannover"); ok {
		t.Error("entry must be a miss after TTL expiry")
	}
}

// TestLocationSearchCacheEviction proves the cache is bounded: exceeding the
// cap evicts the oldest entry.
func TestLocationSearchCacheEviction(t *testing.T) {
	c := newLocationSearchCache(time.Hour, 2)
	clock := newTestClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	c.setNow(clock.Now)

	c.put("a", []LocationResult{{Name: "a"}})
	clock.Advance(time.Second)
	c.put("b", []LocationResult{{Name: "b"}})
	clock.Advance(time.Second)
	c.put("c", []LocationResult{{Name: "c"}}) // exceeds cap 2 -> evicts oldest "a"

	if _, ok := c.get("a"); ok {
		t.Error("oldest entry must be evicted once the cap is exceeded")
	}
	if _, ok := c.get("b"); !ok {
		t.Error("entry b must survive")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("entry c must survive")
	}
}

// TestSearchLocationEnrichmentAndRequest proves AC1a (enrichment query params),
// AC2a (enriched fields) and AC4a (identifying User-Agent) in one pass.
func TestSearchLocationEnrichmentAndRequest(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)

	results, err := svc.SearchLocation("Hannover")
	if err != nil {
		t.Fatalf("SearchLocation error: %v", err)
	}

	// AC1a: outgoing request carries the enrichment params.
	for _, want := range []string{"addressdetails=1", "limit=10", "accept-language=de", "format=json"} {
		if !strings.Contains(rec.lastURL, want) {
			t.Errorf("request URL %q missing %q", rec.lastURL, want)
		}
	}

	// AC4a: identifying User-Agent, not the old stock browser string.
	if rec.lastUA == "" || rec.lastUA == "Mozilla/5.0" {
		t.Errorf("User-Agent = %q, want an identifying UA", rec.lastUA)
	}
	if !strings.Contains(rec.lastUA, "E-INK-Picture") || !strings.Contains(rec.lastUA, "github.com") {
		t.Errorf("User-Agent = %q, want app identifier + contact URL", rec.lastUA)
	}

	// AC2a: enriched fields for the city hit.
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	city := results[0]
	if city.Name != "Hannover" || city.Type != "city" || city.City != "Hannover" ||
		city.Region != "Niedersachsen" || city.Country != "Deutschland" {
		t.Errorf("city enrichment = %+v", city)
	}
	if city.Label != "Hannover, Niedersachsen, Deutschland" {
		t.Errorf("city label = %q", city.Label)
	}

	// town/village and state/county priority orders.
	town := results[1]
	if town.Type != "town" || town.City != "Hanover" || town.Region != "Pennsylvania" {
		t.Errorf("town enrichment = %+v", town)
	}
	village := results[2]
	if village.Type != "village" || village.City != "Beispieldorf" || village.Region != "Landkreis Göttingen" {
		t.Errorf("village enrichment (county fallback) = %+v", village)
	}
}

// TestSearchLocationWizardBackwardCompat proves AC3a: the three legacy fields
// stay populated as strings exactly like the setup wizard expects.
func TestSearchLocationWizardBackwardCompat(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)

	results, err := svc.SearchLocation("Hannover")
	if err != nil {
		t.Fatalf("SearchLocation error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for i, r := range results {
		// Mirror setup-wizard.js: r.display_name && r.lat && r.lon must be set.
		if r.DisplayName == "" || r.Lat == "" || r.Lon == "" {
			t.Errorf("result %d breaks wizard contract: %+v", i, r)
		}
	}
	if results[0].DisplayName != "Hannover, Region Hannover, Niedersachsen, Deutschland" ||
		results[0].Lat != "52.3744779" || results[0].Lon != "9.7385532" {
		t.Errorf("legacy fields altered: %+v", results[0])
	}
}

// TestSearchLocationPostcode proves AC2a for a postcode query: type and
// postcode are set and the label leads with the postcode.
func TestSearchLocationPostcode(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: postcodeLocationJSON}
	svc := newLocationTestService(t, rec)

	results, err := svc.SearchLocation("30159")
	if err != nil {
		t.Fatalf("SearchLocation error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.Type != "postcode" || r.Postcode != "30159" || r.City != "Hannover" {
		t.Errorf("postcode enrichment = %+v", r)
	}
	if r.Label != "30159, Hannover, Niedersachsen, Deutschland" {
		t.Errorf("postcode label = %q", r.Label)
	}
}

// TestSearchLocationOrderPreserved proves AC7a: enrichment keeps Nominatim's
// importance ordering.
func TestSearchLocationOrderPreserved(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)

	results, err := svc.SearchLocation("Hannover")
	if err != nil {
		t.Fatalf("SearchLocation error: %v", err)
	}
	wantOrder := []string{"Hannover", "Hanover", "Beispieldorf"}
	if len(results) != len(wantOrder) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(wantOrder))
	}
	for i, want := range wantOrder {
		if results[i].Name != want {
			t.Errorf("results[%d].Name = %q, want %q", i, results[i].Name, want)
		}
	}
}

// TestSearchLocationCacheCallCount proves AC6a: a repeated query is served from
// cache, so exactly one request goes out to Nominatim.
func TestSearchLocationCacheCallCount(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)

	first, err := svc.SearchLocation("Hannover")
	if err != nil {
		t.Fatalf("first SearchLocation error: %v", err)
	}
	// Whitespace/case variant must hit the same cache key.
	second, err := svc.SearchLocation("  hannover ")
	if err != nil {
		t.Fatalf("second SearchLocation error: %v", err)
	}
	if rec.callCount() != 1 {
		t.Errorf("outgoing Nominatim calls = %d, want 1 (repeat served from cache)", rec.callCount())
	}
	if len(first) != len(second) || first[0].Name != second[0].Name {
		t.Errorf("cache hit returned different results: %+v vs %+v", first, second)
	}
}

// TestSearchLocationRateLimitSpacing proves AC5a: with the limiter's clock
// pinned, a second distinct (uncached) query inside the same 1s window does NOT
// reach the recording transport; advancing the clock past the interval lets the
// next request out. No wall-clock sleeps.
func TestSearchLocationRateLimitSpacing(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)
	clock := newTestClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	svc.geoLimiter.setNow(clock.Now)

	if _, err := svc.SearchLocation("Hamburg"); err != nil {
		t.Fatalf("query 1 error: %v", err)
	}
	if rec.callCount() != 1 {
		t.Fatalf("after query 1, calls = %d, want 1", rec.callCount())
	}
	// Distinct query, same window: limiter must deny -> no outgoing request.
	res2, err := svc.SearchLocation("Bremen")
	if err != nil {
		t.Fatalf("query 2 error: %v", err)
	}
	if rec.callCount() != 1 {
		t.Errorf("distinct query in same 1s window must not go out; calls = %d, want 1", rec.callCount())
	}
	if len(res2) != 0 {
		t.Errorf("rate-limited query must degrade to empty results, got %d", len(res2))
	}
	// Advance past the interval: the next distinct query goes out.
	clock.Advance(nominatimMinInterval)
	if _, err := svc.SearchLocation("Kassel"); err != nil {
		t.Fatalf("query 3 error: %v", err)
	}
	if rec.callCount() != 2 {
		t.Errorf("after advancing past the interval, calls = %d, want 2", rec.callCount())
	}
}

// TestSearchLocationFailOpen proves AC8a: transport error, non-200 and parse
// failure each degrade to an empty slice with no error and no panic.
func TestSearchLocationFailOpen(t *testing.T) {
	cases := []struct {
		name string
		rec  *nominatimRecorder
	}{
		{"transport error", &nominatimRecorder{err: errors.New("dial timeout")}},
		{"non-200", &nominatimRecorder{status: http.StatusServiceUnavailable, body: "boom"}},
		{"bad json", &nominatimRecorder{status: http.StatusOK, body: "not json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newLocationTestService(t, tc.rec)
			results, err := svc.SearchLocation("Hannover")
			if err != nil {
				t.Errorf("fail-open must not return an error, got %v", err)
			}
			if results == nil || len(results) != 0 {
				t.Errorf("fail-open must return an empty (non-nil) slice, got %+v", results)
			}
		})
	}
}

// TestSearchLocationEmptyQuery proves an empty/whitespace query short-circuits
// without any outgoing request.
func TestSearchLocationEmptyQuery(t *testing.T) {
	rec := &nominatimRecorder{status: http.StatusOK, body: canonicalLocationJSON}
	svc := newLocationTestService(t, rec)
	for _, q := range []string{"", "   ", "\t\n"} {
		results, err := svc.SearchLocation(q)
		if err != nil || len(results) != 0 {
			t.Errorf("SearchLocation(%q) = (%v, %v), want empty, nil", q, results, err)
		}
	}
	if rec.callCount() != 0 {
		t.Errorf("empty query must not hit Nominatim; calls = %d", rec.callCount())
	}
}
