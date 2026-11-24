package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/cache"
	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/domain"
)

type memoryStateStore struct {
	state   domain.StoredState
	loadErr error
	saveErr error
	loads   int
	saves   int
}

func (s *memoryStateStore) Load() (domain.StoredState, error) {
	s.loads++
	if s.loadErr != nil {
		return domain.StoredState{}, s.loadErr
	}
	return s.state, nil
}
func (s *memoryStateStore) Save(state domain.StoredState) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = state
	return nil
}

type fakeUsageSource struct {
	reading domain.UsageReading
	err     error
	calls   int
}

func (s *fakeUsageSource) ReadUsage(context.Context) (domain.UsageReading, error) {
	s.calls++
	return s.reading, s.err
}
func (*fakeUsageSource) NextRefreshAllowedAt() time.Time { return time.Time{} }

func syntheticReading(now time.Time) domain.UsageReading {
	used := 0.25
	reset := now.Add(4 * time.Hour)
	return domain.UsageReading{
		ObservedAt: now,
		Quotas: map[domain.QuotaKey]domain.QuotaReading{
			domain.QuotaLimit5h: {Source: domain.SourceRatio, UsedRatio: &used, ResetAt: &reset, Raw: json.RawMessage(`{"used_ratio":0.25}`)},
		},
		Raw:      json.RawMessage(`{"usages":{"limit_5h":{"used_ratio":0.25}},"future_provider_field":"synthetic-passthrough-value"}`),
		Warnings: []domain.Warning{},
	}
}

func jsonDeps(store cache.StateStore, source domain.UsageSource, now time.Time) deps {
	return deps{
		newUsage: func(baseURL, token string) (domain.UsageSource, error) {
			if token == "" {
				return nil, errors.New("empty token")
			}
			return source, nil
		},
		stateStore: store,
		now:        func() time.Time { return now },
	}
}

func runJSONWithDeps(args []string, d deps) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := runWithDeps(args, errorReader{}, &stdout, &stderr, d)
	return code, stdout.String(), stderr.String()
}

func TestJSONLiveSuccessSavesSnapshotAndEmitsOnlyJSON(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	store := &memoryStateStore{state: domain.EmptyStoredState()}
	source := &fakeUsageSource{reading: syntheticReading(now)}
	code, stdout, stderr := runJSONWithDeps([]string{"--json", "--refresh", "30s"}, jsonDeps(store, source, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q stdout=%s", code, stderr, stdout)
	}
	if !strings.HasSuffix(stdout, "}\n") || strings.Contains(stdout, "synthetic-cli-token") {
		t.Fatalf("stdout discipline: %q", stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["schema_version"] != float64(1) || doc["cached"] != false || doc["cache_age_seconds"] != float64(0) {
		t.Errorf("metadata: %+v", doc)
	}
	quotas := doc["quotas"].(map[string]any)
	if len(quotas) != 4 || quotas["limit_7d"] != nil || quotas["limit_month_total"] != nil || quotas["limit_month_code"] != nil {
		t.Errorf("quota nulls: %+v", quotas)
	}
	quota := quotas["limit_5h"].(map[string]any)
	runway := quota["runway"].(map[string]any)
	if runway["status"] != "finite" || runway["basis"] != "window_average" || runway["seconds"] == nil || runway["burn_ratio_per_second"] == nil {
		t.Errorf("runway: %+v", runway)
	}
	raw := doc["raw"].(map[string]any)
	if raw["future_provider_field"] != "synthetic-passthrough-value" {
		t.Errorf("raw passthrough: %+v", raw)
	}
	if store.saves != 1 || store.state.Snapshot == nil || !store.state.Snapshot.GeneratedAt.Equal(now) {
		t.Fatalf("saved state: %+v", store.state)
	}
	if source.calls != 1 {
		t.Errorf("reads=%d", source.calls)
	}
}

func TestJSONFetchFailureUsesWarnedCachedSnapshot(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	snapshot := syntheticSnapshot(now.Add(-time.Minute))
	store := &memoryStateStore{state: domain.StoredState{CacheVersion: 1, Snapshot: &snapshot, Samples: map[domain.QuotaKey][]domain.Sample{}}}
	source := &fakeUsageSource{err: errors.New("fetch failed for synthetic-cli-token")}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, source, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var doc struct {
		Cached      bool             `json:"cached"`
		Age         int64            `json:"cache_age_seconds"`
		GeneratedAt string           `json:"generated_at"`
		Warnings    []domain.Warning `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Cached || doc.Age != 60 || doc.GeneratedAt != now.Add(-time.Minute).Format(time.RFC3339) {
		t.Errorf("cache metadata: %+v", doc)
	}
	if len(doc.Warnings) != 1 || doc.Warnings[0].Section != "Refresh" || strings.Contains(doc.Warnings[0].Message, "synthetic-cli-token") || !strings.Contains(doc.Warnings[0].Message, "[redacted]") {
		t.Errorf("warnings: %+v", doc.Warnings)
	}
	if store.saves != 0 {
		t.Error("cached fallback must not rewrite the snapshot")
	}
}

func TestJSONFetchFailureWithoutCacheHasNoStdout(t *testing.T) {
	isolatedEnv(t)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	store := &memoryStateStore{state: domain.EmptyStoredState()}
	source := &fakeUsageSource{err: errors.New("fetch failed for synthetic-cli-token")}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, source, time.Now()))
	if code != exitFailure || stdout != "" {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	if strings.Contains(stderr, "synthetic-cli-token") || !strings.Contains(stderr, "[redacted]") {
		t.Errorf("stderr=%q", stderr)
	}
}

func TestJSONMissingTokenNeverPrompts(t *testing.T) {
	isolatedEnv(t)
	store := &memoryStateStore{state: domain.EmptyStoredState()}
	source := &fakeUsageSource{}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, source, time.Now()))
	if code != exitCredentials || stdout != "" {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
	if !strings.Contains(stderr, "KIMIBURN_TOKEN") || !strings.Contains(stderr, "kimiburn setup") {
		t.Errorf("stderr=%q", stderr)
	}
	if source.calls != 0 || store.loads != 0 {
		t.Errorf("unexpected I/O: reads=%d cache loads=%d", source.calls, store.loads)
	}
}

func TestJSONLiveCacheSaveFailureAddsWarningWithoutFailing(t *testing.T) {
	isolatedEnv(t)
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	store := &memoryStateStore{state: domain.EmptyStoredState(), saveErr: errors.New("cannot save synthetic-cli-token")}
	source := &fakeUsageSource{reading: syntheticReading(now)}
	code, stdout, stderr := runJSONWithDeps([]string{"--json"}, jsonDeps(store, source, now))
	if code != exitOK || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "synthetic-cli-token") || !strings.Contains(stdout, `"section": "Cache"`) || !strings.Contains(stdout, "[redacted]") {
		t.Errorf("stdout=%s", stdout)
	}
}

func TestJSONNoCacheNeverReadsOrWritesState(t *testing.T) {
	home := isolatedEnv(t)
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	t.Setenv("KIMIBURN_TOKEN", "synthetic-cli-token")
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := syntheticSnapshot(now.Add(-time.Minute))
	if err := cache.NewStore(paths.CacheFile).Save(domain.StoredState{CacheVersion: 1, Snapshot: &snapshot}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(paths.CacheFile)
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeUsageSource{err: errors.New("offline")}
	code, stdout, stderr := runJSONWithDeps([]string{"--json", "--no-cache"}, jsonDeps(nil, source, now))
	if code != exitFailure || stdout != "" || !strings.Contains(stderr, "offline") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	after, err := os.ReadFile(paths.CacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("--no-cache modified state")
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "kimiburn")); err != nil {
		t.Fatal(err)
	}
}

func TestJSONRejectsInvalidRefreshAndCommandCombination(t *testing.T) {
	isolatedEnv(t)
	for _, args := range [][]string{{"--json", "--refresh", "1s"}, {"--json", "setup"}} {
		code, stdout, _ := runJSONWithDeps(args, deps{})
		if code != exitUsage || stdout != "" {
			t.Errorf("%v: exit=%d stdout=%q", args, code, stdout)
		}
	}
}

func syntheticSnapshot(generated time.Time) domain.Snapshot {
	reading := syntheticReading(generated)
	snapshot := domain.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated, Quotas: reading.Quotas,
		Runways: map[domain.QuotaKey]domain.RunwayEstimate{domain.QuotaLimit5h: {Status: "unavailable", Reason: "insufficient_samples", Confidence: "unavailable"}},
		Raw:     reading.Raw, Warnings: []domain.Warning{},
	}
	return snapshot
}
