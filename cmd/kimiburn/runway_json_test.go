package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func TestJSONUsesPriorCachedSamplesForRunway(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	reset := now.Add(2 * time.Hour)
	samples := make([]domain.Sample, 4)
	for i := range samples {
		samples[i] = domain.Sample{ObservedAt: now.Add(time.Duration(i-4) * 30 * time.Second), UsedRatio: .2 + float64(i)*.01, ResetAt: reset}
	}
	store := &memoryStateStore{state: domain.StoredState{CacheVersion: 1, Samples: map[domain.QuotaKey][]domain.Sample{domain.QuotaLimit5h: samples}}}
	reading := syntheticReading(now)
	used := .24
	reading.Quotas[domain.QuotaLimit5h] = domain.QuotaReading{UsedRatio: &used, ResetAt: &reset}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, &fakeUsageSource{reading: reading}, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var doc struct {
		Quotas map[string]struct {
			Runway domain.RunwayEstimate `json:"runway"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Quotas[string(domain.QuotaLimit5h)].Runway
	if got.Status != "finite" || got.SampleCount != 5 || got.WindowSeconds != 120 || got.Confidence != "low" || got.ExhaustedAt == nil {
		t.Fatalf("runway=%+v", got)
	}
	if len(store.state.Samples[domain.QuotaLimit5h]) != 5 {
		t.Fatalf("samples=%+v", store.state.Samples)
	}
}

func TestJSONNoCacheColdStartReportsInsufficientSamples(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	store := &memoryStateStore{state: domain.EmptyStoredState()}
	code, stdout, stderr := runJSONWithDeps([]string{"--json", "--no-cache"}, jsonDeps(store, &fakeUsageSource{reading: syntheticReading(now)}, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var doc struct {
		Quotas map[string]struct {
			Runway domain.RunwayEstimate `json:"runway"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Quotas[string(domain.QuotaLimit5h)].Runway
	if got.Status != "unavailable" || got.Reason != "insufficient_samples" || got.SampleCount != 1 {
		t.Fatalf("runway=%+v", got)
	}
	if store.loads != 0 || store.saves != 0 {
		t.Fatalf("no-cache I/O loads=%d saves=%d", store.loads, store.saves)
	}
}
