package services

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// safeFetchSentinelToken is a fake token; the URL must never contain it.
const safeFetchSentinelToken = "SENTINEL_TOKEN_DO_NOT_LOG"

// recordingTransport records every request (URL + Authorization header) and
// returns a canned response or error. It is the stub for the header/redirect
// assertions.
type recordingTransport struct {
	mu       sync.Mutex
	requests []recordedRequest
	respond  func() (*http.Response, error)
}

type recordedRequest struct {
	url  string
	auth string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.requests = append(rt.requests, recordedRequest{
		url:  req.URL.String(),
		auth: req.Header.Get("Authorization"),
	})
	rt.mu.Unlock()
	return rt.respond()
}

func (rt *recordingTransport) count() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.requests)
}

func respondOK(body string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
}

func respondRedirect(location string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		h := make(http.Header)
		h.Set("Location", location)
		return &http.Response{
			StatusCode: http.StatusFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     h,
		}, nil
	}
}

// constReader yields an endless stream of one byte, for the oversized-body cap.
type constReader byte

func (c constReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

// testAllowlistClient builds a client pinned to host:port but swaps its
// transport for rt, keeping the redirect-disabled policy and timeout in effect.
func testAllowlistClient(host, port string, rt http.RoundTripper) *allowlistClient {
	c := newAllowlistClient(host, port)
	c.http.Transport = rt
	return c
}

// TestSafeFetchTokenOnlyInBearerHeader is AC-SEC4: the token is sent only as
// the Authorization header, never in the URL/query.
func TestSafeFetchTokenOnlyInBearerHeader(t *testing.T) {
	rt := &recordingTransport{respond: respondOK(`{"state":"21.5"}`)}
	c := testAllowlistClient("10.0.0.5", "8123", rt)

	target := "http://10.0.0.5:8123/api/states/sensor.x"
	body, status, err := safeFetchAllowlisted(context.Background(), c, target, safeFetchSentinelToken, hassFetchBodyLimit)
	if err != nil {
		t.Fatalf("safeFetchAllowlisted: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(string(body), "21.5") {
		t.Fatalf("body = %q, want the stub payload", body)
	}
	if rt.count() != 1 {
		t.Fatalf("transport calls = %d, want 1", rt.count())
	}
	got := rt.requests[0]
	if got.auth != "Bearer "+safeFetchSentinelToken {
		t.Errorf("Authorization = %q, want %q", got.auth, "Bearer "+safeFetchSentinelToken)
	}
	if strings.Contains(got.url, safeFetchSentinelToken) {
		t.Errorf("request URL %q must NOT contain the token", got.url)
	}
}

// TestSafeFetchRejectsNonHTTPScheme is AC-SEC7(b): a non-http(s) target is
// rejected without any transport call.
func TestSafeFetchRejectsNonHTTPScheme(t *testing.T) {
	for _, target := range []string{
		"file:///etc/passwd",
		"gopher://10.0.0.5:8123/",
		"ftp://10.0.0.5/x",
	} {
		rt := &recordingTransport{respond: respondOK("")}
		c := testAllowlistClient("10.0.0.5", "8123", rt)
		if _, _, err := safeFetchAllowlisted(context.Background(), c, target, "", hassFetchBodyLimit); err == nil {
			t.Errorf("safeFetchAllowlisted(%q) = nil error, want rejection", target)
		}
		if rt.count() != 0 {
			t.Errorf("safeFetchAllowlisted(%q) made %d transport calls, want 0 (no dial)", target, rt.count())
		}
	}
}

// TestSafeFetchRejectsOffAllowlistHostNoDial is AC-SEC6: a target host:port
// different from the pinned one is refused BEFORE dialing (DialContext never
// called).
func TestSafeFetchRejectsOffAllowlistHostNoDial(t *testing.T) {
	var dials int32
	tr := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			atomic.AddInt32(&dials, 1)
			return nil, errors.New("dial should never be reached")
		},
	}
	c := &allowlistClient{
		host: "10.0.0.5",
		port: "8123",
		http: &http.Client{
			Transport:     tr,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}

	for _, target := range []string{
		"http://evil.example:8123/api/states/x",
		"http://10.0.0.5:9999/api/states/x",
		"https://10.0.0.5/api/states/x", // default https port 443 ≠ allowlisted 8123
	} {
		if _, _, err := safeFetchAllowlisted(context.Background(), c, target, "", hassFetchBodyLimit); err == nil {
			t.Errorf("safeFetchAllowlisted(%q) = nil error, want off-allowlist rejection", target)
		}
	}
	if n := atomic.LoadInt32(&dials); n != 0 {
		t.Errorf("DialContext called %d times, want 0 (rejection must precede dial)", n)
	}
}

// TestSafeFetchDefaultPortNormalization is AC-SEC6 (both directions): the
// host:port comparison normalizes the default port, so http://h and
// http://h:80 are equal.
func TestSafeFetchDefaultPortNormalization(t *testing.T) {
	cases := []struct {
		name       string
		clientHost string
		clientPort string
		target     string
	}{
		{"client default port, target explicit :80", "10.0.0.5", "80", "http://10.0.0.5:80/api/states/x"},
		{"client explicit :80, target default port", "10.0.0.5", "80", "http://10.0.0.5/api/states/x"},
		{"https default port, target explicit :443", "10.0.0.5", "443", "https://10.0.0.5:443/api/states/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &recordingTransport{respond: respondOK("{}")}
			c := testAllowlistClient(tc.clientHost, tc.clientPort, rt)
			if _, status, err := safeFetchAllowlisted(context.Background(), c, tc.target, "", hassFetchBodyLimit); err != nil || status != http.StatusOK {
				t.Errorf("%s: err=%v status=%d, want nil/200 (normalized match)", tc.name, err, status)
			}
		})
	}
}

// TestSafeFetchDoesNotFollowRedirect is AC-SEC5: a 302 to another host is NOT
// followed; the 302 is returned and the foreign host is never requested.
func TestSafeFetchDoesNotFollowRedirect(t *testing.T) {
	rt := &recordingTransport{respond: respondRedirect("http://evil.example/")}
	c := testAllowlistClient("10.0.0.5", "8123", rt)

	target := "http://10.0.0.5:8123/api/states/sensor.x"
	_, status, err := safeFetchAllowlisted(context.Background(), c, target, "", hassFetchBodyLimit)
	if err != nil {
		t.Fatalf("safeFetchAllowlisted: %v", err)
	}
	if status != http.StatusFound {
		t.Errorf("status = %d, want 302 (redirect returned, not followed)", status)
	}
	if rt.count() != 1 {
		t.Errorf("transport calls = %d, want 1 (redirect must not be followed)", rt.count())
	}
	for _, req := range rt.requests {
		if strings.Contains(req.url, "evil.example") {
			t.Errorf("transport was called for the redirect target %q, want never", req.url)
		}
	}
}

// TestSafeFetchCapsBodyAt2MiB is AC-SEC8: an oversized body is capped at 2 MiB.
func TestSafeFetchCapsBodyAt2MiB(t *testing.T) {
	respond := func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.LimitReader(constReader('a'), 5<<20)), // 5 MiB
			Header:     make(http.Header),
		}, nil
	}
	rt := &recordingTransport{respond: respond}
	c := testAllowlistClient("10.0.0.5", "8123", rt)

	body, status, err := safeFetchAllowlisted(context.Background(), c, "http://10.0.0.5:8123/api/states/x", "", hassFetchBodyLimit)
	if err != nil {
		t.Fatalf("safeFetchAllowlisted: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if int64(len(body)) > hassFetchBodyLimit {
		t.Errorf("body length = %d, want <= %d (2 MiB cap)", len(body), hassFetchBodyLimit)
	}
	if int64(len(body)) != hassFetchBodyLimit {
		t.Errorf("body length = %d, want exactly the 2 MiB cap for a 5 MiB source", len(body))
	}
}

// TestSafeFetchGenericErrorOnTransportFailure asserts a transport error is
// converted to the generic errFetchUnavailable (no host/token leak).
func TestSafeFetchGenericErrorOnTransportFailure(t *testing.T) {
	rt := &recordingTransport{respond: func() (*http.Response, error) {
		return nil, errors.New("dial tcp 10.0.0.5:8123: connection refused")
	}}
	c := testAllowlistClient("10.0.0.5", "8123", rt)

	_, _, err := safeFetchAllowlisted(context.Background(), c, "http://10.0.0.5:8123/api/states/x", safeFetchSentinelToken, hassFetchBodyLimit)
	if !errors.Is(err, errFetchUnavailable) {
		t.Fatalf("err = %v, want errFetchUnavailable", err)
	}
	if strings.Contains(err.Error(), "10.0.0.5") || strings.Contains(err.Error(), safeFetchSentinelToken) {
		t.Errorf("returned error %q leaks host/token detail", err.Error())
	}
}

// TestPinnedDialContextRefusesOtherHost asserts the dial pin (defense in depth)
// refuses any address other than the pinned host:port.
func TestPinnedDialContextRefusesOtherHost(t *testing.T) {
	dc := pinnedDialContext("10.0.0.5", "8123")

	if _, err := dc(context.Background(), "tcp", "evil.example:8123"); !errors.Is(err, errDialNotAllowed) {
		t.Errorf("dial to off-pin host = %v, want errDialNotAllowed", err)
	}
	if _, err := dc(context.Background(), "tcp", "10.0.0.5:9999"); !errors.Is(err, errDialNotAllowed) {
		t.Errorf("dial to off-pin port = %v, want errDialNotAllowed", err)
	}
	// The pinned address passes the pin check and proceeds to a real dial (which
	// fails with a network error, NOT the pin error).
	if _, err := dc(context.Background(), "tcp", "10.0.0.5:8123"); errors.Is(err, errDialNotAllowed) {
		t.Error("dial to pinned host was refused by the pin, want it to pass the pin check")
	}
}
