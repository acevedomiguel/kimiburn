package app

import (
	"errors"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/ui"
)

func TestScheduleDelay(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	tests := []struct {
		name     string
		interval time.Duration
		m        Model
		want     time.Duration
		allowed  bool
	}{
		{"initial30", 30 * time.Second, Model{}, 30 * time.Second, true},
		{"zeroAutomatic", 0, Model{}, 0, true},
		{"normal", 30 * time.Second, Model{lastAttempt: now.Add(-15 * time.Second)}, 30 * time.Second, true},
		{"backoff", 30 * time.Second, Model{lastAttempt: now.Add(-15 * time.Second), nextAllowed: now.Add(time.Minute)}, time.Minute, true},
		{"cacheOnly", 30 * time.Second, Model{state: ViewReady, cached: true}, 30 * time.Second, false},
		{"setup", 30 * time.Second, Model{state: ViewSetup}, 30 * time.Second, false},
		{"suspended", 30 * time.Second, Model{state: ViewReady, suspended: true}, 30 * time.Second, false},
		{"inflight", 30 * time.Second, Model{state: ViewReady, inFlight: true}, 30 * time.Second, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, allowed := test.m.ScheduleDelay(now, test.interval)
			if got != test.want || allowed != test.allowed {
				t.Fatalf("delay=%v allowed=%v want %v/%v", got, allowed, test.want, test.allowed)
			}
		})
	}
}

func TestStartAttemptThrottlesAndCommitsGeneration(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	m := NewModel(Options{Now: func() time.Time { return now }})
	m.lastAttempt = now.Add(-15 * time.Second)
	cmd, started := m.StartAttempt(false)
	if started || cmd != nil || m.nextAllowed != now.Add(15*time.Second) {
		t.Fatalf("premature automatic start=%v next=%v", started, m.nextAllowed)
	}
	m.nextAllowed = time.Time{}
	m.lastAttempt = now.Add(-31 * time.Second)
	cmd, started = m.StartAttempt(false)
	if !started || cmd == nil || m.generation != 1 || m.manual != 0 || !m.inFlight {
		t.Fatalf("automatic attempt=%+v", m)
	}
	if m.nextAllowed != now.Add(30*time.Second) {
		t.Fatalf("automatic next=%v", m.nextAllowed)
	}

	m.inFlight = false
	m.lastAttempt = now.Add(-31 * time.Second)
	m.nextAllowed = time.Time{}
	m.interval = 0
	cmd, started = m.StartAttempt(true)
	if !started || cmd == nil || m.manual != 1 || m.nextAllowed != now.Add(30*time.Second) {
		t.Fatalf("manual attempt=%+v next=%v", m, m.nextAllowed)
	}

	m.inFlight = false
	m.suspended = true
	cmd, started = m.StartAttempt(false)
	if started || cmd != nil {
		t.Fatal("suspended automatic attempt started")
	}
}

func TestCacheRulesAndWarnings(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	warm := NewModel(Options{Now: func() time.Time { return now }})
	warm.snapshot, warm.cached = ui.SyntheticSnapshot(), true
	warm = warm.applyCache(CacheResult{Err: errors.New("read failed")})
	if warm.state != ViewReady || warm.warning == "" {
		t.Fatalf("warm cache failure state=%v warning=%q", warm.state, warm.warning)
	}
	if cmd, ok := warm.StartAttempt(true); !ok || cmd == nil {
		t.Fatal("warm cache read failure blocked refresh")
	}

	cold := NewModel(Options{Now: func() time.Time { return now }})
	cold = cold.applyCache(CacheResult{Err: errors.New("missing")})
	if cold.state != ViewFatal || cold.notice == "" {
		t.Fatalf("cold cache failure=%+v", cold)
	}
}

func mustNow() time.Time { return time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC) }
