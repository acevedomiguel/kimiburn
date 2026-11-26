package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

func TestJSONUsesPriorCachedSamplesForRunway(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	reset := now.Add(2 * time.Hour)
	samples := make([]kimi.Sample, 4)
	for i := range samples {
		samples[i] = kimi.Sample{ObservedAt: now.Add(time.Duration(i-4) * 30 * time.Second), UsedRatio: .2 + float64(i)*.01, ResetAt: reset}
	}
	store := &memoryStateStore{state: kimi.StoredState{CacheVersion: 1, Samples: map[kimi.QuotaKey][]kimi.Sample{kimi.QuotaLimit5h: samples}}}
	reading := syntheticReading(now)
	used := .24
	reading.Quotas[kimi.QuotaLimit5h] = kimi.QuotaReading{UsedRatio: &used, ResetAt: &reset}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, &fakeUsageSource{reading: reading}, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var doc struct {
		Quotas map[string]struct {
			Runway kimi.RunwayEstimate `json:"runway"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Quotas[string(kimi.QuotaLimit5h)].Runway
	if got.Status != "finite" || got.SampleCount != 5 || got.WindowSeconds != 120 || got.Confidence != "low" || got.ExhaustedAt == nil {
		t.Fatalf("runway=%+v", got)
	}
	if len(store.state.Samples[kimi.QuotaLimit5h]) != 5 {
		t.Fatalf("samples=%+v", store.state.Samples)
	}
}

func TestJSONNoCacheColdStartUsesWindowAverageWithoutStateIO(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	store := &memoryStateStore{state: kimi.EmptyStoredState()}
	code, stdout, stderr := runJSONWithDeps([]string{"--json", "--no-cache"}, jsonDeps(store, &fakeUsageSource{reading: syntheticReading(now)}, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var doc struct {
		Quotas map[string]struct {
			Runway kimi.RunwayEstimate `json:"runway"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Quotas[string(kimi.QuotaLimit5h)].Runway
	if got.Status != "finite" || got.Basis != "window_average" || got.SampleCount != 1 || got.BurnRatioPerSecond == nil {
		t.Fatalf("runway=%+v", got)
	}
	if store.loads != 0 || store.saves != 0 {
		t.Fatalf("no-cache I/O loads=%d saves=%d", store.loads, store.saves)
	}
}
