package kimiburn_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kimiburn "github.com/acevedomiguel/kimiburn"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewSourceUsesInjectedTransportAndClock(t *testing.T) {
	fixed := time.Date(2026, 11, 25, 9, 30, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/usages" {
			http.NotFound(w, request)
			return
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	client := server.Client()
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	transportCalls := 0
	client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		transportCalls++
		return transport.RoundTrip(request)
	})
	source, err := kimiburn.NewSource(
		server.URL,
		"synthetic-library-token",
		kimiburn.WithHTTPClient(client),
		kimiburn.WithClock(func() time.Time { return fixed }),
	)
	if err != nil {
		t.Fatal(err)
	}
	reading, err := source.ReadUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || transportCalls != 1 {
		t.Fatalf("requests=%d transport calls=%d, want 1/1", requests, transportCalls)
	}
	if !reading.ObservedAt.Equal(fixed) {
		t.Fatalf("ObservedAt=%s, want %s", reading.ObservedAt, fixed)
	}
}

func TestNewSourceRefusesRedirects(t *testing.T) {
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests++
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	defer redirect.Close()

	source, err := kimiburn.NewSource(redirect.URL, "synthetic-library-token", kimiburn.WithHTTPClient(redirect.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.ReadUsage(context.Background())
	if !errors.Is(err, kimiburn.ErrRedirect) {
		t.Fatalf("ReadUsage error=%v, want ErrRedirect", err)
	}
	if targetRequests != 0 {
		t.Fatalf("redirect target requests=%d, want 0", targetRequests)
	}
}

func TestNewSourceRejectsMissingTokenAndUnsupportedBase(t *testing.T) {
	_, err := kimiburn.NewSource(kimiburn.DefaultBaseURL, " \t ")
	if !errors.Is(err, kimiburn.ErrMissingToken) {
		t.Fatalf("empty token error=%v, want ErrMissingToken", err)
	}
	_, err = kimiburn.NewSource("https://example.com/coding/v1", "synthetic-library-token")
	if !errors.Is(err, kimiburn.ErrUnsupportedBaseURL) {
		t.Fatalf("unsupported base error=%v, want ErrUnsupportedBaseURL", err)
	}
}

func TestNewSourceNilClockUsesDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	source, err := kimiburn.NewSource(server.URL, "synthetic-library-token", kimiburn.WithHTTPClient(server.Client()), kimiburn.WithClock(nil))
	if err != nil {
		t.Fatal(err)
	}
	reading, err := source.ReadUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reading.ObservedAt.IsZero() {
		t.Fatal("ObservedAt is zero with the default clock")
	}
}

func TestNewSourceNilHTTPClientUsesDefault(t *testing.T) {
	source, err := kimiburn.NewSource(kimiburn.DefaultBaseURL, "synthetic-library-token", kimiburn.WithHTTPClient(nil))
	if err != nil || source == nil {
		t.Fatalf("NewSource() source=%v error=%v", source, err)
	}
}
