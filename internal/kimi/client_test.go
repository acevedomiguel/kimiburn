package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/config"
)

// syntheticToken is invented for tests only. It must never appear in an error,
// a log line, or a fixture committed to the repository.
const syntheticToken = "km_synthetic_test_token_0123456789abcdef"

// fixedNow is the injected clock instant every deterministic test observes.
var fixedNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

// sleepRecorder captures retry delays so tests never actually wait. It reports
// the context error so a canceled context still stops the retry loop.
type sleepRecorder struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (s *sleepRecorder) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.delays = append(s.delays, d)
	s.mu.Unlock()
	return ctx.Err()
}

func (s *sleepRecorder) recorded() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.delays...)
}

// requestCounter counts attempts so tests can prove bounded retries.
type requestCounter struct{ n int32 }

func (c *requestCounter) add() { atomic.AddInt32(&c.n, 1) }

func (c *requestCounter) count() int { return int(atomic.LoadInt32(&c.n)) }

// newTestClient builds a client against a loopback httptest server with a
// deterministic clock, jitter, and sleeper.
func newTestClient(t *testing.T, h http.Handler, opts ...Option) (*Client, *httptest.Server, *sleepRecorder) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	sleeps := &sleepRecorder{}
	base := []Option{
		WithBaseURL(srv.URL),
		WithToken(syntheticToken),
		WithVersion("test"),
		WithSleep(sleeps.sleep),
		WithJitter(func() float64 { return 0 }),
		WithClock(func() time.Time { return fixedNow }),
	}
	c, err := NewClient(append(base, opts...)...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv, sleeps
}

func TestNewClientRejectsDisallowedBaseURLs(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		wantErr bool
	}{
		{"primary", config.DefaultBaseURL, false},
		{"overseas", config.OverseasBaseURL, false},
		{"primary trailing slash", config.DefaultBaseURL + "/", false},
		{"plain http production", "http://api.kimi.com/coding/v1", true},
		{"open platform", "https://api.moonshot.ai/v1", true},
		{"openrouter", "https://openrouter.ai/api/v1", true},
		{"wrong path", "https://api.kimi.com/coding/v2", true},
		{"query string", "https://api.kimi.com/coding/v1?token=leak", true},
		{"userinfo", "https://token@api.kimi.com/coding/v1", true},
		{"loopback http", "http://127.0.0.1:1", false},
		{"loopback localhost", "http://localhost:1", false},
		{"loopback ipv6", "http://[::1]:1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(WithBaseURL(tc.base), WithToken(syntheticToken))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewClient(%q) = nil error, want a refusal", tc.base)
				}
				if !errors.Is(err, ErrUnsupportedBaseURL) {
					t.Fatalf("error = %v, want ErrUnsupportedBaseURL", err)
				}
				if strings.Contains(err.Error(), "leak") {
					t.Errorf("refusal must not echo the configured base URL: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient(%q) = %v, want success", tc.base, err)
			}
			if got, want := c.usageURL(), strings.TrimRight(tc.base, "/")+"/usages"; got != want {
				t.Errorf("usageURL = %q, want %q", got, want)
			}
		})
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	if _, err := NewClient(WithBaseURL(config.DefaultBaseURL)); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("error = %v, want ErrMissingToken", err)
	}
}

func TestUsageURLUsesOnlyTheSelectedProductionBase(t *testing.T) {
	for _, base := range []string{config.DefaultBaseURL, config.OverseasBaseURL} {
		c, err := NewClient(WithBaseURL(base), WithToken(syntheticToken))
		if err != nil {
			t.Fatalf("NewClient(%q): %v", base, err)
		}
		if got, want := c.usageURL(), base+"/usages"; got != want {
			t.Errorf("usageURL = %q, want %q", got, want)
		}
		if strings.Contains(c.usageURL(), "moonshot") || strings.Contains(c.usageURL(), "openrouter") {
			t.Errorf("usageURL = %q, must never target another product", c.usageURL())
		}
	}
}

func TestReadUsageSendsExactRequest(t *testing.T) {
	body := `{"usages":{"limit_5h":{"used_ratio":0.25}},"goods_version":"v1","unknown":{"nested":true}}`
	var calls requestCounter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add()
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/usages" {
			t.Errorf("path = %q, want /usages", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want none", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+syntheticToken {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "Kimiburn/9.9.9" {
			t.Errorf("user agent = %q", got)
		}
		if r.ContentLength > 0 {
			t.Errorf("content length = %d, want no request body", r.ContentLength)
		}
		io.WriteString(w, body)
	}))
	defer srv.Close()

	c, err := NewClient(
		WithBaseURL(srv.URL),
		WithToken(syntheticToken),
		WithVersion("9.9.9"),
		WithClock(func() time.Time { return fixedNow }),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reading, err := c.ReadUsage(context.Background())
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := calls.count(); got != 1 {
		t.Errorf("requests = %d, want exactly 1", got)
	}
	if !reading.ObservedAt.Equal(fixedNow) {
		t.Errorf("observed at = %v, want the injected clock instant", reading.ObservedAt)
	}
	if string(reading.Raw) != body {
		t.Errorf("raw = %s, want the exact top-level object %s", reading.Raw, body)
	}
	if len(reading.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", reading.Warnings)
	}
}

func TestReadUsageAcceptsObjectWithoutQuotas(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"usages":{}}`)
	}))
	reading, err := c.ReadUsage(context.Background())
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if len(reading.Raw) == 0 {
		t.Error("raw must retain the top-level object")
	}
}

func TestReadUsageRejectsNonObjectTopLevel(t *testing.T) {
	bodies := []string{"", "   ", "[]", "[{}]", `"text"`, "123", "null", "true"}
	for _, body := range bodies {
		t.Run(strconv.Quote(body), func(t *testing.T) {
			c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, body)
			}))
			_, err := c.ReadUsage(context.Background())
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("body %q: error = %v, want ErrMalformedResponse", body, err)
			}
		})
	}
}

func TestReadUsageRejectsTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"a":1}{"b":2}`, `{"a":1} trailing`, `{"a":1}]`} {
		t.Run(strconv.Quote(body), func(t *testing.T) {
			c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, body)
			}))
			if _, err := c.ReadUsage(context.Background()); !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("body %q: error = %v, want ErrMalformedResponse", body, err)
			}
		})
	}
}

func TestReadUsageAcceptsTrailingWhitespace(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "{\"a\":1}\n\n")
	}))
	reading, err := c.ReadUsage(context.Background())
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if string(reading.Raw) != `{"a":1}` {
		t.Errorf("raw = %q, want the object without surrounding whitespace", reading.Raw)
	}
}

func TestReadUsageRejectsOversizedBody(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), maxResponseBytes+1))
	}))
	_, err := c.ReadUsage(context.Background())
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestReadUsageRetriesRetryableStatuses(t *testing.T) {
	for _, status := range []int{429, 502, 503, 504} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls requestCounter
			c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.add(); calls.count() == 1 {
					w.WriteHeader(status)
					return
				}
				io.WriteString(w, `{"usages":{}}`)
			}))
			if _, err := c.ReadUsage(context.Background()); err != nil {
				t.Fatalf("ReadUsage: %v", err)
			}
			if got := calls.count(); got != 2 {
				t.Errorf("requests = %d, want 2 (one retry)", got)
			}
			if got := len(sleeps.recorded()); got != 1 {
				t.Errorf("sleeps = %d, want 1", got)
			}
		})
	}
}

func TestReadUsageStopsAfterTwoRetries(t *testing.T) {
	var calls requestCounter
	c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	_, err := c.ReadUsage(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("error = %v, want ErrServer", err)
	}
	if got := calls.count(); got != 3 {
		t.Errorf("requests = %d, want 3 (initial attempt plus two retries)", got)
	}
	if got := len(sleeps.recorded()); got != 2 {
		t.Errorf("sleeps = %d, want 2", got)
	}
}

func TestReadUsageDoesNotRetryNonRetryableStatuses(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{400, ErrUnexpectedStatus},
		{401, ErrUnauthorized},
		{403, ErrForbidden},
		{404, ErrNotFound},
		{405, ErrUnexpectedStatus},
		{409, ErrUnexpectedStatus},
		{500, ErrServer},
		{501, ErrServer},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			var calls requestCounter
			c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.add()
				w.WriteHeader(tc.status)
			}))
			_, err := c.ReadUsage(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got := calls.count(); got != 1 {
				t.Errorf("requests = %d, want 1 (no retry)", got)
			}
			if got := len(sleeps.recorded()); got != 0 {
				t.Errorf("sleeps = %v, want none", sleeps.recorded())
			}
		})
	}
}

func TestReadUsageRetriesTransportErrors(t *testing.T) {
	var calls requestCounter
	real := http.DefaultTransport
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"usages":{}}`)
	}), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.add(); calls.count() == 1 {
			return nil, errors.New("synthetic dial failure")
		}
		return real.RoundTrip(r)
	})}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := calls.count(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReadUsageHonorsRetryAfterSeconds(t *testing.T) {
	var calls requestCounter
	c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := sleeps.recorded(); len(got) != 1 || got[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s]", got)
	}
}

func TestReadUsageHonorsRetryAfterHTTPDate(t *testing.T) {
	var calls requestCounter
	c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("Retry-After", fixedNow.Add(3*time.Second).Format(http.TimeFormat))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := sleeps.recorded(); len(got) != 1 || got[0] != 3*time.Second {
		t.Fatalf("sleeps = %v, want [3s]", got)
	}
}

func TestReadUsageFallsBackToJitteredBackoffForInvalidRetryAfter(t *testing.T) {
	var calls requestCounter
	c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("Retry-After", "later")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := sleeps.recorded(); len(got) != 1 || got[0] != 125*time.Millisecond {
		t.Fatalf("sleeps = %v, want [125ms] with zero jitter", got)
	}
}

func TestBackoffStartsAt250msAndCapsAt4s(t *testing.T) {
	c := &Client{jitter: func() float64 { return 0 }}
	if got := c.backoff(0); got != 125*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 125ms with zero jitter", got)
	}
	if got := c.backoff(1); got != 250*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 250ms with zero jitter", got)
	}
	if got := c.backoff(10); got != 2*time.Second {
		t.Errorf("backoff(10) = %v, want the 4s cap halved to 2s", got)
	}
	c.jitter = func() float64 { return 1 }
	if got := c.backoff(10); got != 4*time.Second {
		t.Errorf("backoff(10) = %v, want the 4s cap", got)
	}
}

func TestReadUsageRecordsRateLimitReset(t *testing.T) {
	reset := fixedNow.Add(90 * time.Second)
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		io.WriteString(w, `{"usages":{}}`)
	}))
	if got := c.NextRefreshAllowedAt(); !got.IsZero() {
		t.Fatalf("NextRefreshAllowedAt before any read = %v, want zero", got)
	}
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := c.NextRefreshAllowedAt(); !got.Equal(reset) {
		t.Errorf("NextRefreshAllowedAt = %v, want %v", got, reset)
	}
}

func TestReadUsageRecordsRetryAfterAsNextRefresh(t *testing.T) {
	var calls requestCounter
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got, want := c.NextRefreshAllowedAt(), fixedNow.Add(5*time.Second); !got.Equal(want) {
		t.Errorf("NextRefreshAllowedAt = %v, want %v", got, want)
	}
}

func TestNextRefreshAllowedAtIsConcurrencySafe(t *testing.T) {
	reset := fixedNow.Add(90 * time.Second)
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		io.WriteString(w, `{"usages":{}}`)
	}))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := c.ReadUsage(context.Background()); err != nil {
				t.Errorf("ReadUsage: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_ = c.NextRefreshAllowedAt()
		}()
	}
	wg.Wait()
	if got := c.NextRefreshAllowedAt(); !got.Equal(reset) {
		t.Errorf("NextRefreshAllowedAt = %v, want %v", got, reset)
	}
}

func TestReadUsageStopsOnCancellation(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.ReadUsage(ctx)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadUsage did not return after cancellation")
	}
}

func TestReadUsageNeverSleepsPastCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls requestCounter
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add()
		w.WriteHeader(http.StatusServiceUnavailable)
	}), WithSleep(func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}))
	_, err := c.ReadUsage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := calls.count(); got != 1 {
		t.Errorf("requests = %d, want 1 (no retry after cancellation)", got)
	}
}

func TestReadUsageEnforcesPerAttemptTimeout(t *testing.T) {
	var calls requestCounter
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add()
		<-r.Context().Done()
	}), WithTimeout(20*time.Millisecond))
	_, err := c.ReadUsage(context.Background())
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, want ErrTransport", err)
	}
	if got := calls.count(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestReadUsageRefusesRedirect(t *testing.T) {
	var redirected requestCounter
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.add()
		io.WriteString(w, `{"usages":{}}`)
	}))
	defer other.Close()

	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", other.URL+"/usages")
		w.WriteHeader(http.StatusFound)
	}))
	_, err := c.ReadUsage(context.Background())
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("error = %v, want ErrRedirect", err)
	}
	if got := redirected.count(); got != 0 {
		t.Errorf("redirect target received %d requests, want 0", got)
	}
	if strings.Contains(err.Error(), other.URL) {
		t.Errorf("error must not expose the redirect target: %v", err)
	}
	if strings.Contains(err.Error(), syntheticToken) {
		t.Errorf("error must not expose the token: %v", err)
	}
}

func TestReadUsagePreservesRawUnknownFields(t *testing.T) {
	body := `{"usages":{"limit_7d":{"used_ratio":0.5}},"boosterWallet":{"amount":1},"future_field":[1,2,3]}`
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	reading, err := c.ReadUsage(context.Background())
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(reading.Raw, &got); err != nil {
		t.Fatalf("raw is not valid JSON: %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("raw = %v, want %v", got, want)
	}
}

// A server body, header, or redirect target must never survive into an error:
// the typed error is safe to print. The synthetic secret is invented for this
// test only.
func TestReadUsageErrorsAreSafeAndRedacted(t *testing.T) {
	const secret = "km_server_secret_should_never_leak"
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrForbidden},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"server error", http.StatusInternalServerError, ErrServer},
		{"unexpected status", http.StatusTeapot, ErrUnexpectedStatus},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Server-Secret", secret)
				w.WriteHeader(tc.status)
				io.WriteString(w, `{"error":"`+secret+`"}`)
			}))
			_, err := c.ReadUsage(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			text := err.Error()
			for _, leaked := range []string{secret, syntheticToken, "Authorization"} {
				if strings.Contains(text, leaked) {
					t.Errorf("error leaked %q: %v", leaked, text)
				}
			}
			if !strings.Contains(text, "/usages") {
				t.Errorf("error should name the endpoint path: %v", text)
			}
			if !strings.Contains(text, strconv.Itoa(tc.status)) {
				t.Errorf("error should include the status class: %v", text)
			}
		})
	}
}

func TestReadUsageMalformedErrorIsSafe(t *testing.T) {
	const secret = "km_malformed_secret_should_never_leak"
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"error":"`+secret+`"`)
	}))
	_, err := c.ReadUsage(context.Background())
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), syntheticToken) {
		t.Errorf("malformed error leaked response data: %v", err)
	}
}

func TestReadUsageDoesNotRetryMalformedOrOversizedResponses(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want error
	}{
		{"malformed", []byte(`{"a":1`), ErrMalformedResponse},
		{"oversized", bytes.Repeat([]byte("x"), maxResponseBytes+1), ErrResponseTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls requestCounter
			c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.add()
				w.Write(tc.body)
			}))
			_, err := c.ReadUsage(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got := calls.count(); got != 1 {
				t.Errorf("requests = %d, want 1 (no retry)", got)
			}
			if got := len(sleeps.recorded()); got != 0 {
				t.Errorf("sleeps = %v, want none", sleeps.recorded())
			}
		})
	}
}

func TestReadUsageRejectsEmptySuccessBody(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if _, err := c.ReadUsage(context.Background()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

func TestReadUsageDoesNotRequestWhenAlreadyCanceled(t *testing.T) {
	var calls requestCounter
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add()
		io.WriteString(w, `{"usages":{}}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ReadUsage(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := calls.count(); got != 0 {
		t.Errorf("requests = %d, want 0", got)
	}
}

// The earliest allowed refresh is the latest constraint observed, so a later
// response cannot pull the next refresh earlier.
func TestNextRefreshAllowedAtKeepsLatestConstraint(t *testing.T) {
	far := fixedNow.Add(120 * time.Second)
	near := fixedNow.Add(60 * time.Second)
	var calls requestCounter
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(far.Unix(), 10))
		} else {
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(near.Unix(), 10))
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := c.NextRefreshAllowedAt(); !got.Equal(far) {
		t.Errorf("NextRefreshAllowedAt = %v, want the later constraint %v", got, far)
	}
}

func TestNextRefreshAllowedAtAcceptsMilliseconds(t *testing.T) {
	reset := fixedNow.Add(90 * time.Second)
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.UnixMilli(), 10))
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := c.NextRefreshAllowedAt(); !got.Equal(reset) {
		t.Errorf("NextRefreshAllowedAt = %v, want %v", got, reset)
	}
}

func TestNextRefreshAllowedAtIgnoresInvalidReset(t *testing.T) {
	for _, value := range []string{"soon", "", "0"} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if value != "" {
					w.Header().Set("X-RateLimit-Reset", value)
				}
				io.WriteString(w, `{"usages":{}}`)
			}))
			if _, err := c.ReadUsage(context.Background()); err != nil {
				t.Fatalf("ReadUsage: %v", err)
			}
			if got := c.NextRefreshAllowedAt(); !got.IsZero() {
				t.Errorf("NextRefreshAllowedAt = %v, want zero for %q", got, value)
			}
		})
	}
}

// A Retry-After date that has already passed means "retry now", so no extra
// delay is invented.
func TestReadUsageDoesNotDelayForPastRetryAfterDate(t *testing.T) {
	var calls requestCounter
	c, _, sleeps := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.add(); calls.count() == 1 {
			w.Header().Set("Retry-After", fixedNow.Add(-5*time.Second).Format(http.TimeFormat))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	if _, err := c.ReadUsage(context.Background()); err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if got := sleeps.recorded(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("sleeps = %v, want [0s]", got)
	}
}

func TestNewClientRejectsEmptyToken(t *testing.T) {
	for _, token := range []string{"", "   "} {
		if _, err := NewClient(WithBaseURL(config.DefaultBaseURL), WithToken(token)); !errors.Is(err, ErrMissingToken) {
			t.Fatalf("token %q: error = %v, want ErrMissingToken", token, err)
		}
	}
}

// The raw object must keep the provider's own bytes, including internal
// whitespace and key order, so the cache and --json output round-trip it.
func TestReadUsagePreservesRawBytesExactly(t *testing.T) {
	body := `{"usages": {"limit_5h": {"used_ratio": 0.25, "reset_time": "2026-11-24T17:00:00Z"}}, "goods_version": "v1"}`
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	reading, err := c.ReadUsage(context.Background())
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if string(reading.Raw) != body {
		t.Errorf("raw = %s, want the provider bytes unchanged %s", reading.Raw, body)
	}
}

// A loopback override is a test-only allowance; the two production bases are
// the only non-loopback hosts the client will ever build a URL for.
func TestNewClientAllowsOnlyLoopbackOverrides(t *testing.T) {
	for _, base := range []string{"https://api.kimi.com.evil.test/coding/v1", "http://10.0.0.1/coding/v1", "https://api.kimi.ai/coding/v1/extra"} {
		if _, err := NewClient(WithBaseURL(base), WithToken(syntheticToken)); !errors.Is(err, ErrUnsupportedBaseURL) {
			t.Errorf("NewClient(%q) error = %v, want ErrUnsupportedBaseURL", base, err)
		}
	}
}
