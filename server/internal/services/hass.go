package services

// HassService is the read-only Home-Assistant fetch layer (specs/B5-home-
// assistant.md, sub-task B5a). It reads the admin connection from a
// hass.Manager and fetches entity state through the hardened safe-fetch helper
// (allowlist, redirects off, size cap). It performs NO write/control calls
// (no /api/services/...): the widget only displays. The token flows only into
// the Authorization header inside safeFetchAllowlisted and is never logged or
// placed in a URL — failures log the host only.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"e-ink-picture/server/internal/hass"
)

// HA fetch outcomes surfaced to the content layer. They are distinct so the
// caller (B5b fillHassContent) can map 404 to "Unbekannt: <id>" while transport
// errors and other non-200s map to the generic "Nicht verfügbar".
var (
	// ErrHassNotConfigured means no base URL and/or token is set.
	ErrHassNotConfigured = errors.New("hass not configured")
	// ErrHassEntityUnknown means the entity returned HTTP 404.
	ErrHassEntityUnknown = errors.New("hass entity unknown")
	// ErrHassUnavailable is the generic failure (transport error, timeout,
	// non-200 except 404). It carries no host/IP/token detail.
	ErrHassUnavailable = errors.New("hass unavailable")
)

// HassEntity is the subset of a Home-Assistant state object the widget needs.
type HassEntity struct {
	EntityID     string
	State        string
	Unit         string
	FriendlyName string
}

// HassService fetches HA entity state read-only. The allowlist client is built
// lazily per configured host:port and rebuilt when the config changes.
type HassService struct {
	mgr *hass.Manager

	mu        sync.Mutex
	client    *allowlistClient
	clientKey string
	// testTransport, when non-nil, replaces the pinned client's transport for
	// tests (the client's redirect policy and body cap stay in effect). It is
	// never set in production.
	testTransport http.RoundTripper
}

// NewHassService creates a HassService backed by mgr.
func NewHassService(mgr *hass.Manager) *HassService {
	return &HassService{mgr: mgr}
}

// hassStateResponse is the parsed subset of GET /api/states/<entity_id>.
type hassStateResponse struct {
	EntityID   string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		Unit         string `json:"unit_of_measurement"`
		FriendlyName string `json:"friendly_name"`
	} `json:"attributes"`
}

// FetchEntity fetches the state of one entity read-only. It returns
// ErrHassNotConfigured when no connection is set, ErrHassEntityUnknown on 404,
// and ErrHassUnavailable for transport errors / timeouts / other non-200s. A
// generic failure is negatively cached per host+entity (negativeCacheTTL) so a
// second render inside the window makes no network call.
func (s *HassService) FetchEntity(ctx context.Context, entityID string) (*HassEntity, error) {
	base, token, ok := s.mgr.Config()
	if !ok {
		return nil, ErrHassNotConfigured
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, ErrHassUnavailable
	}
	host, port := hostPort(u)
	if host == "" || port == "" {
		return nil, ErrHassUnavailable
	}

	negKey := "hass:" + host + "|" + entityID
	if failCache.blocked(negKey) {
		return nil, ErrHassUnavailable
	}

	target := strings.TrimRight(base, "/") + "/api/states/" + url.PathEscape(entityID)
	client := s.clientFor(host, port)

	body, status, err := safeFetchAllowlisted(ctx, client, target, token, hassFetchBodyLimit)
	if err != nil {
		slog.Warn("hass fetch failed", "host", host)
		failCache.markFailure(negKey)
		return nil, ErrHassUnavailable
	}

	switch status {
	case http.StatusOK:
		// parsed below
	case http.StatusNotFound:
		// A typo'd/deleted entity is a legitimate response, not a transport
		// failure: do not negatively cache it (the entity may reappear).
		return nil, ErrHassEntityUnknown
	default:
		slog.Warn("hass fetch non-200", "host", host, "status", status)
		failCache.markFailure(negKey)
		return nil, ErrHassUnavailable
	}

	var raw hassStateResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		slog.Warn("hass response parse failed", "host", host)
		failCache.markFailure(negKey)
		return nil, ErrHassUnavailable
	}
	failCache.markSuccess(negKey)

	return &HassEntity{
		EntityID:     entityID,
		State:        raw.State,
		Unit:         raw.Attributes.Unit,
		FriendlyName: raw.Attributes.FriendlyName,
	}, nil
}

// clientFor returns the pinned allowlist client for host:port, building it
// lazily and rebuilding it when the configured host:port changes.
func (s *HassService) clientFor(host, port string) *allowlistClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := net.JoinHostPort(host, port)
	if s.client == nil || s.clientKey != key {
		s.client = newAllowlistClient(host, port)
		s.clientKey = key
	}
	if s.testTransport != nil {
		s.client.http.Transport = s.testTransport
	}
	return s.client
}
