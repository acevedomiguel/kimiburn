package cache_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/cache"
	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func float64Ptr(value float64) *float64 { return &value }

func syntheticState(ratio float64) domain.StoredState {
	generated := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	reset := generated.Add(90 * time.Minute)
	return domain.StoredState{
		CacheVersion: domain.CacheSchemaVersion,
		Snapshot: &domain.Snapshot{
			SchemaVersion: domain.SnapshotSchemaVersion,
			GeneratedAt:   generated,
			Quotas: map[domain.QuotaKey]domain.QuotaReading{
				domain.QuotaLimit5h: {
					Source: domain.SourceRatio, UsedRatio: float64Ptr(ratio), ResetAt: &reset,
					Raw: json.RawMessage(`{"used_ratio":0.25,"future_quota_field":{"kept":true}}`),
				},
			},
			Runways: map[domain.QuotaKey]domain.RunwayEstimate{
				domain.QuotaLimit5h: {Status: "unavailable", Reason: "insufficient_samples", SampleCount: 0},
			},
			GoodsVersion:  json.RawMessage(`{"future_plan":"synthetic"}`),
			BoosterWallet: json.RawMessage(`null`),
			Raw:           json.RawMessage(`{"usages":{"limit_5h":{"used_ratio":0.25}},"unknown_provider_object":{"zero":0,"text":"synthetic"}}`),
			Warnings:      []domain.Warning{{Section: "Synthetic", Message: "redacted synthetic warning"}},
		},
		Samples: map[domain.QuotaKey][]domain.Sample{
			domain.QuotaLimit5h: {{ObservedAt: generated, UsedRatio: ratio, ResetAt: reset}},
		},
	}
}

func TestStoreRoundTripReplacementPermissionsAndRawSurvival(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store := cache.NewStore(path)
	original := syntheticState(0.25)
	if err := store.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("cache mode = %o, want 600", got)
		}
		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if got := dirInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("cache directory mode = %o, want 700", got)
		}
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.CacheVersion != domain.CacheSchemaVersion || loaded.Snapshot == nil {
		t.Fatalf("Load() invalid state: %+v", loaded)
	}
	if !loaded.Snapshot.GeneratedAt.Equal(original.Snapshot.GeneratedAt) {
		t.Fatalf("GeneratedAt = %v, want %v", loaded.Snapshot.GeneratedAt, original.Snapshot.GeneratedAt)
	}
	quota := loaded.Snapshot.Quotas[domain.QuotaLimit5h]
	if quota.UsedRatio == nil || *quota.UsedRatio != 0.25 {
		t.Fatalf("UsedRatio = %v, want 0.25", quota.UsedRatio)
	}
	if len(loaded.Samples[domain.QuotaLimit5h]) != 1 {
		t.Fatalf("samples = %d, want 1", len(loaded.Samples[domain.QuotaLimit5h]))
	}
	assertJSONEqual(t, loaded.Snapshot.Raw, original.Snapshot.Raw)
	assertJSONEqual(t, loaded.Snapshot.Quotas[domain.QuotaLimit5h].Raw, original.Snapshot.Quotas[domain.QuotaLimit5h].Raw)
	assertJSONEqual(t, loaded.Snapshot.GoodsVersion, original.Snapshot.GoodsVersion)

	replacement := syntheticState(0.50)
	if err := store.Save(replacement); err != nil {
		t.Fatalf("replacement Save() error = %v", err)
	}
	loaded, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := *loaded.Snapshot.Quotas[domain.QuotaLimit5h].UsedRatio; got != 0.50 {
		t.Fatalf("replacement UsedRatio = %v, want 0.50", got)
	}
}

func TestLoadMissingReturnsEmptyV1State(t *testing.T) {
	state, err := cache.NewStore(filepath.Join(t.TempDir(), "missing", "state.json")).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.CacheVersion != domain.CacheSchemaVersion || state.Snapshot != nil || state.Samples == nil || len(state.Samples) != 0 {
		t.Fatalf("Load() = %+v, want empty v1 state", state)
	}
}

func TestLoadRejectsMalformedAndWrongVersionWithoutRawDataInErrors(t *testing.T) {
	const secret = "synthetic-secret-that-must-not-leak"
	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "malformed", body: `{"cache_version":1,"raw":"` + secret + `"`, want: cache.ErrMalformed},
		{name: "wrong version", body: `{"cache_version":2,"raw":{"token":"` + secret + `"}}`, want: cache.ErrUnsupportedVersion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := cache.NewStore(path).Load()
			if !errors.Is(err, test.want) {
				t.Fatalf("Load() error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Load() error leaked raw data: %v", err)
			}
		})
	}
}

func TestSaveRejectsWrongVersionAndCleansTemporaryFilesOnFailure(t *testing.T) {
	t.Run("wrong version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		state := syntheticState(0.25)
		state.CacheVersion = 99
		if err := cache.NewStore(path).Save(state); !errors.Is(err, cache.ErrUnsupportedVersion) {
			t.Fatalf("Save() error = %v, want ErrUnsupportedVersion", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cache file exists after rejected save: %v", err)
		}
	})

	t.Run("replacement failure cleans temporary file and redacts error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
		state := syntheticState(0.25)
		state.Snapshot.Raw = json.RawMessage(`{"secret":"synthetic-secret-that-must-not-leak"}`)
		err := cache.NewStore(path).Save(state)
		if err == nil {
			t.Fatal("Save() error = nil, want replacement failure")
		}
		if strings.Contains(err.Error(), "synthetic-secret-that-must-not-leak") {
			t.Fatalf("Save() error leaked raw data: %v", err)
		}
		matches, globErr := filepath.Glob(filepath.Join(dir, ".state-*.tmp"))
		if globErr != nil {
			t.Fatal(globErr)
		}
		if len(matches) != 0 {
			t.Fatalf("temporary files remain: %v", matches)
		}
	})
}

func TestDisabledStorePerformsNoIOAndReturnsEmptyState(t *testing.T) {
	store := cache.NewDisabledStore()
	state, err := store.Load()
	if err != nil {
		t.Fatalf("disabled Load() error = %v", err)
	}
	if state.CacheVersion != domain.CacheSchemaVersion || state.Snapshot != nil {
		t.Fatalf("disabled Load() = %+v", state)
	}
	if err := store.Save(syntheticState(0.75)); err != nil {
		t.Fatalf("disabled Save() error = %v", err)
	}
}

func assertJSONEqual(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("invalid got JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("invalid want JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %v, want %v", gotValue, wantValue)
	}
}
