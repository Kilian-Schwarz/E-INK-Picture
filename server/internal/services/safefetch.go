package services

// Hardened, allowlist-based safe-fetch helper for the Home-Assistant fetch
// path (specs/B5-home-assistant.md, sub-task B5a). It is deliberately separate
// from defaultHTTPClient (which follows redirects and has no host allowlist):
// a later ticket can place a block-list variant next to it to harden the
// existing user-URL widgets (calendar/news/custom) — this helper does NOT
// touch them.
//
// Controls (defense in depth):
//   - scheme restricted to http/https (rejected before any dial),
//   - target host:port pinned to the configured allowlist entry, compared with
//     default-port normalization (http→80, https→443), rejected before any
//     dial,
//   - a DialContext that connects ONLY to the pinned host:port (rebinding
//     defense — for a future hostname config the resolved IP must be pinned,
//     see spec §2 / AC-SEC6),
//   - redirects DISABLED (CheckRedirect → http.ErrUseLastResponse): a 3xx is
//     returned as-is, never followed to another host,
//   - a request timeout and an io.LimitReader body cap,
//   - the bearer token sent ONLY as an Authorization header, never in the URL.
//
// All returned errors are generic (errFetchUnavailable): no host, IP, token or
// internal detail leaks to the caller, which surfaces it to the UI.

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// hassFetchBodyLimit caps the response body read for the HA fetch path.
const hassFetchBodyLimit int64 = 2 << 20 // 2 MiB

// safeFetchTimeout bounds a single allowlisted request end to end.
const safeFetchTimeout = 10 * time.Second

// errFetchUnavailable is the single generic error returned by the safe-fetch
// helper. It intentionally carries no host/IP/token/internal detail so a
// caller can surface it without leaking anything.
var errFetchUnavailable = errors.New("upstream unavailable")

// errDialNotAllowed is the internal DialContext refusal (off-pin address). It
// never reaches a caller of safeFetchAllowlisted — it is wrapped by the
// transport and converted to errFetchUnavailable — but lets the dial pin be
// asserted in isolation.
var errDialNotAllowed = errors.New("dial address not allowed")

// allowlistClient bundles an http.Client pinned to exactly one host:port with
// the normalized allowlist entry, so safeFetchAllowlisted can reject an
// off-allowlist target BEFORE dialing (the DialContext pin is the second,
// independent control). The token is never stored here.
type allowlistClient struct {
	http *http.Client
	host string // normalized allowed host (no port)
	port string // normalized allowed port (default-port filled in)
}

// newAllowlistClient builds a client pinned to host:port: redirects disabled,
// a request timeout and a DialContext that refuses any other address. host and
// port must already be normalized (default port filled in by hostPort).
func newAllowlistClient(host, port string) *allowlistClient {
	return &allowlistClient{
		host: host,
		port: port,
		http: &http.Client{
			Timeout: safeFetchTimeout,
			// Redirects disabled: a 3xx is returned as the response, never
			// followed (SSRF: no redirect to a non-allowlisted host).
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DialContext:           pinnedDialContext(host, port),
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          2,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   safeFetchTimeout,
				ExpectContinueTimeout: time.Second,
			},
		},
	}
}

// pinnedDialContext returns a DialContext that connects ONLY to host:port and
// refuses any other address with errDialNotAllowed (defense in depth against
// DNS rebinding). The comparison is against the exact pinned host:port string
// the transport derives from the (already allowlist-checked) target URL.
func pinnedDialContext(host, port string) func(context.Context, string, string) (net.Conn, error) {
	pinned := net.JoinHostPort(host, port)
	d := &net.Dialer{Timeout: safeFetchTimeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != pinned {
			return nil, errDialNotAllowed
		}
		return d.DialContext(ctx, network, pinned)
	}
}

// hostPort extracts the host and port from a URL, normalizing the default port
// (http→80, https→443) so http://h and http://h:80 compare equal.
func hostPort(u *url.URL) (host, port string) {
	host = u.Hostname()
	port = u.Port()
	if port != "" {
		return host, port
	}
	switch u.Scheme {
	case "https":
		return host, "443"
	case "http":
		return host, "80"
	}
	return host, ""
}

// safeFetchAllowlisted GETs targetURL through c with the hardened controls and
// returns (body, statusCode, nil) on a completed request or a generic error.
// The bearer token, when non-empty, is sent ONLY as an Authorization header —
// never in the URL or query. The scheme and host:port allowlist checks run
// before any dial; a rejected target returns errFetchUnavailable without
// touching the network.
func safeFetchAllowlisted(ctx context.Context, c *allowlistClient, targetURL, bearer string, limit int64) ([]byte, int, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, 0, errFetchUnavailable
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, 0, errFetchUnavailable
	}

	host, port := hostPort(u)
	if host == "" || port == "" {
		return nil, 0, errFetchUnavailable
	}
	if host != c.host || port != c.port {
		return nil, 0, errFetchUnavailable
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, errFetchUnavailable
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Generic: the underlying error may name the host/IP; never surface it.
		return nil, 0, errFetchUnavailable
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, 0, errFetchUnavailable
	}
	return body, resp.StatusCode, nil
}
