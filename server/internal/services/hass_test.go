package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"e-ink-picture/server/internal/hass"
)

const hassTestBaseURL = "http://10.0.0.5:8123"

// newConfiguredHassService builds a HassService configured with the sentinel
// token and the given test transport, resetting the package negative cache.
func newConfiguredHassService(t *testing.T, transport http.RoundTripper) *HassService {
	t.Helper()
	failCache.reset()
	t.Cleanup(failCache.reset)

	mgr, err := hass.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.SetConfig(hassTestBaseURL, safeFetchSentinelToken); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	svc := NewHassService(mgr)
	svc.testTransport = transport
	return svc
}

// TestFetchEntityNotConfigured: with no config the fetch returns
// ErrHassNotConfigured and never touches the network.
func TestFetchEntityNotConfigured(t *testing.T) {
	failCache.reset()
	t.Cleanup(failCache.reset)

	mgr, err := hass.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	rt := &recordingTransport{respond: respondOK("{}")}
	svc := NewHassService(mgr)
	svc.testTransport = rt

	if _, err := svc.FetchEntity(context.Background(), "sensor.x"); !errors.Is(err, ErrHassNotConfigured) {
		t.Fatalf("FetchEntity = %v, want ErrHassNotConfigured", err)
	}
	if rt.count() != 0 {
		t.Errorf("transport called %d times, want 0 when not configured", rt.count())
	}
}

// TestFetchEntity200 parses a 200 response and confirms the token rides only in
// the Authorization header (never the URL), with the entity id path-escaped.
func TestFetchEntity200(t *testing.T) {
	const payload = `{"entity_id":"sensor.wohnzimmer","state":"21.5",` +
		`"attributes":{"unit_of_measurement":"°C","friendly_name":"Wohnzimmer"}}`
	rt := &recordingTransport{respond: respondOK(payload)}
	svc := newConfiguredHassService(t, rt)

	ent, err := svc.FetchEntity(context.Background(), "sensor.wohnzimmer")
	if err != nil {
		t.Fatalf("FetchEntity: %v", err)
	}
	if ent.State != "21.5" || ent.Unit != "°C" || ent.FriendlyName != "Wohnzimmer" {
		t.Errorf("entity = %+v, want state 21.5 / unit °C / name Wohnzimmer", ent)
	}
	if ent.EntityID != "sensor.wohnzimmer" {
		t.Errorf("EntityID = %q, want sensor.wohnzimmer", ent.EntityID)
	}
	if rt.count() != 1 {
		t.Fatalf("transport calls = %d, want 1", rt.count())
	}
	got := rt.requests[0]
	wantURL := hassTestBaseURL + "/api/states/sensor.wohnzimmer"
	if got.url != wantURL {
		t.Errorf("request URL = %q, want %q", got.url, wantURL)
	}
	if got.auth != "Bearer "+safeFetchSentinelToken {
		t.Errorf("Authorization = %q, want Bearer <sentinel>", got.auth)
	}
	if strings.Contains(got.url, safeFetchSentinelToken) {
		t.Errorf("request URL %q must NOT contain the token", got.url)
	}
}

// TestFetchEntityPathEscape: an entity id with a special char is path-escaped
// and the token still never lands in the URL.
func TestFetchEntityPathEscape(t *testing.T) {
	rt := &recordingTransport{respond: respondOK(`{"state":"on"}`)}
	svc := newConfiguredHassService(t, rt)

	if _, err := svc.FetchEntity(context.Background(), "sensor.a b"); err != nil {
		t.Fatalf("FetchEntity: %v", err)
	}
	got := rt.requests[0].url
	if !strings.Contains(got, "/api/states/sensor.a%20b") {
		t.Errorf("request URL = %q, want path-escaped entity id", got)
	}
}

// TestFetchEntity404: a 404 maps to ErrHassEntityUnknown and is NOT negatively
// cached (the entity may reappear), so a retry hits the network again.
func TestFetchEntity404(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       http.NoBody,
			Header:     make(http.Header),
		}, nil
	}}
	svc := newConfiguredHassService(t, rt)

	if _, err := svc.FetchEntity(context.Background(), "sensor.missing"); !errors.Is(err, ErrHassEntityUnknown) {
		t.Fatalf("FetchEntity 404 = %v, want ErrHassEntityUnknown", err)
	}
	if _, err := svc.FetchEntity(context.Background(), "sensor.missing"); !errors.Is(err, ErrHassEntityUnknown) {
		t.Fatalf("FetchEntity 404 retry = %v, want ErrHassEntityUnknown", err)
	}
	if rt.count() != 2 {
		t.Errorf("transport calls = %d, want 2 (404 must not be negatively cached)", rt.count())
	}
}

// TestFetchEntityTransportErrorNegativeCache: a transport failure maps to the
// generic ErrHassUnavailable and is negatively cached, so a second fetch inside
// the TTL makes no new network attempt.
func TestFetchEntityTransportErrorNegativeCache(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	svc := newConfiguredHassService(t, rt)

	if _, err := svc.FetchEntity(context.Background(), "sensor.x"); !errors.Is(err, ErrHassUnavailable) {
		t.Fatalf("FetchEntity = %v, want ErrHassUnavailable", err)
	}
	if _, err := svc.FetchEntity(context.Background(), "sensor.x"); !errors.Is(err, ErrHassUnavailable) {
		t.Fatalf("FetchEntity (cached) = %v, want ErrHassUnavailable", err)
	}
	if rt.count() != 1 {
		t.Errorf("transport calls = %d, want 1 (negative cache must suppress the retry)", rt.count())
	}
}

// TestFetchEntityNon200NegativeCache: a non-200/non-404 status maps to the
// generic ErrHassUnavailable and is negatively cached.
func TestFetchEntityNon200NegativeCache(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       http.NoBody,
			Header:     make(http.Header),
		}, nil
	}}
	svc := newConfiguredHassService(t, rt)

	if _, err := svc.FetchEntity(context.Background(), "sensor.x"); !errors.Is(err, ErrHassUnavailable) {
		t.Fatalf("FetchEntity = %v, want ErrHassUnavailable", err)
	}
	if _, err := svc.FetchEntity(context.Background(), "sensor.x"); !errors.Is(err, ErrHassUnavailable) {
		t.Fatalf("FetchEntity (cached) = %v, want ErrHassUnavailable", err)
	}
	if rt.count() != 1 {
		t.Errorf("transport calls = %d, want 1 (negative cache must suppress the retry)", rt.count())
	}
}

// TestFetchEntityNeverLogsToken is AC-SEC2 (runtime, fetch path): the token
// never appears in any log line across the success and failure paths.
func TestFetchEntityNeverLogsToken(t *testing.T) {
	buf := captureLogs(t)

	// Failure path (logs the host).
	failRT := &recordingTransport{respond: func() (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	failSvc := newConfiguredHassService(t, failRT)
	if _, err := failSvc.FetchEntity(context.Background(), "sensor.x"); err == nil {
		t.Fatal("expected failure")
	}

	// Success path.
	okRT := &recordingTransport{respond: respondOK(`{"state":"21.5"}`)}
	okSvc := newConfiguredHassService(t, okRT)
	if _, err := okSvc.FetchEntity(context.Background(), "sensor.y"); err != nil {
		t.Fatalf("FetchEntity: %v", err)
	}

	if strings.Contains(buf.String(), safeFetchSentinelToken) {
		t.Fatalf("token sentinel leaked into logs:\n%s", buf.String())
	}
}
