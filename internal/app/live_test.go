package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
	"github.com/acevedomiguel/kimiburn/internal/kimi"
)

type memoryStateStore struct {
	state      domain.StoredState
	loadErr    error
	saveErr    error
	loads      int
	saves      int
	savedState domain.StoredState
}

func (s *memoryStateStore) Load() (domain.StoredState, error) {
	s.loads++
	return s.state, s.loadErr
}
func (s *memoryStateStore) Save(state domain.StoredState) error {
	s.saves++
	s.savedState = state
	return s.saveErr
}

type liveUsageSource struct {
	reading domain.UsageReading
	err     error
	next    time.Time
}

func (s liveUsageSource) ReadUsage(context.Context) (domain.UsageReading, error) {
	return s.reading, s.err
}
func (s liveUsageSource) NextRefreshAllowedAt() time.Time { return s.next }

func TestLiveOptionsCacheConfigurationAndSuccess(t *testing.T) {
	now := time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)
	cached := domain.Snapshot{SchemaVersion: 1, GeneratedAt: now.Add(-time.Minute), Quotas: map[domain.QuotaKey]domain.QuotaReading{}, Runways: map[domain.QuotaKey]domain.RunwayEstimate{}, Warnings: []domain.Warning{}}
	samples := map[domain.QuotaKey][]domain.Sample{domain.QuotaLimit5h: {{ObservedAt: now.Add(-time.Minute), UsedRatio: .2, ResetAt: now.Add(time.Hour)}}}
	store := &memoryStateStore{state: domain.StoredState{CacheVersion: 1, Snapshot: &cached, Samples: samples}}
	next := now.Add(90 * time.Second)
	source := liveUsageSource{next: next, reading: domain.UsageReading{Raw: json.RawMessage(`{"future":true}`), Quotas: map[domain.QuotaKey]domain.QuotaReading{}}}
	opts, cancel, err := NewLiveModelOptions(LiveOptions{
		BaseURL: "https://api.kimi.com/coding/v1", Interval: 30 * time.Second, Store: store, Now: func() time.Time { return now },
		ResolveToken: func(context.Context) (string, error) { return "test-secret-token", nil },
		NewUsage: func(base, token string) (domain.UsageSource, error) {
			if token != "test-secret-token" || base != "https://api.kimi.com/coding/v1" {
				t.Fatalf("factory args %q %q", base, token)
			}
			return source, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if opts.Snapshot.GeneratedAt.IsZero() || opts.Interval != 30*time.Second || !opts.IntervalSet {
		t.Fatalf("live options=%+v", opts)
	}
	if configured, err := opts.Configured(); err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	result := opts.Refresh(context.Background(), RefreshRequest{Generation: 1})
	if result.Err != nil || result.Snapshot.GeneratedAt.IsZero() || !result.NextAllowed.Equal(next) {
		t.Fatalf("result=%+v", result)
	}
	if store.saves != 1 || len(store.savedState.Samples[domain.QuotaLimit5h]) != 1 {
		t.Fatalf("saved=%+v", store.savedState)
	}
	if string(result.Snapshot.Raw) != `{"future":true}` {
		t.Fatalf("raw=%s", result.Snapshot.Raw)
	}
}

func TestLiveOptionsFailuresAreRedactedAndClassified(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		auth bool
	}{
		{"unauthorized", kimi.ErrUnauthorized, true}, {"forbidden", kimi.ErrForbidden, true}, {"transport", kimi.ErrTransport, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryStateStore{state: domain.EmptyStoredState()}
			opts, cancel, err := NewLiveModelOptions(LiveOptions{Store: store, ResolveToken: func(context.Context) (string, error) { return "never-print-test-secret", nil }, NewUsage: func(string, string) (domain.UsageSource, error) { return liveUsageSource{err: test.err}, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			result := opts.Refresh(context.Background(), RefreshRequest{})
			if result.AuthFailure != test.auth || !errors.Is(result.Err, test.err) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
	store := &memoryStateStore{state: domain.EmptyStoredState(), saveErr: errors.New("never-print-test-secret")}
	opts, cancel, err := NewLiveModelOptions(LiveOptions{Store: store, ResolveToken: func(context.Context) (string, error) { return "never-print-test-secret", nil }, NewUsage: func(string, string) (domain.UsageSource, error) {
		return liveUsageSource{reading: domain.UsageReading{Raw: json.RawMessage(`{}`), Quotas: map[domain.QuotaKey]domain.QuotaReading{}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	result := opts.Refresh(context.Background(), RefreshRequest{})
	if len(result.Snapshot.Warnings) != 1 || containsString(result.Snapshot.Warnings[0].Message, "never-print-test-secret") {
		t.Fatalf("warnings=%+v", result.Snapshot.Warnings)
	}
}

func TestLiveOptionsCacheFailureAndNoCache(t *testing.T) {
	store := &memoryStateStore{state: domain.EmptyStoredState(), loadErr: errors.New("bad cache")}
	opts, cancel, err := NewLiveModelOptions(LiveOptions{Store: store, NoCache: true, ResolveToken: func(context.Context) (string, error) { return "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if store.loads != 0 || opts.Snapshot.GeneratedAt.IsZero() == false {
		t.Fatalf("cache used: loads=%d opts=%+v", store.loads, opts)
	}
	configured, err := opts.Configured()
	if err != nil || configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	result := opts.Refresh(context.Background(), RefreshRequest{})
	if !result.AuthFailure || result.Err == nil {
		t.Fatalf("missing token result=%+v", result)
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
