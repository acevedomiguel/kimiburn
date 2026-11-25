package kimiburn_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	kimiburn "github.com/acevedomiguel/kimiburn"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteJSONUsesPublicSchema(t *testing.T) {
	now := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	generated := now.Add(-65 * time.Second)
	reset := generated.Add(time.Hour)
	used := 0.0
	snapshot := kimiburn.Snapshot{
		SchemaVersion: kimiburn.SnapshotSchemaVersion,
		GeneratedAt:   generated,
		Quotas: map[kimiburn.QuotaKey]kimiburn.QuotaReading{
			kimiburn.QuotaLimit5h: {Source: kimiburn.SourceRatio, UsedRatio: &used, ResetAt: &reset},
		},
		Runways: map[kimiburn.QuotaKey]kimiburn.RunwayEstimate{
			kimiburn.QuotaLimit5h: {Status: "no_consumption", Confidence: "low"},
		},
		Raw:      json.RawMessage(`{"future_provider_field":"synthetic-value"}`),
		Warnings: []kimiburn.Warning{},
	}
	var output bytes.Buffer
	err := kimiburn.WriteJSON(&output, snapshot, kimiburn.JSONOptions{
		APIBase: kimiburn.DefaultBaseURL,
		Cached:  true,
		Now:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(output.String(), "}\n") {
		t.Fatalf("JSON lacks exactly one trailing newline: %q", output.String())
	}
	var document struct {
		SchemaVersion   int                        `json:"schema_version"`
		CacheAgeSeconds int64                      `json:"cache_age_seconds"`
		Quotas          map[string]json.RawMessage `json:"quotas"`
		Raw             map[string]any             `json:"raw"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || document.CacheAgeSeconds != 65 {
		t.Fatalf("schema=%d cache age=%d, want 1/65", document.SchemaVersion, document.CacheAgeSeconds)
	}
	var quota struct {
		UsedRatio *float64 `json:"used_ratio"`
	}
	if err := json.Unmarshal(document.Quotas["limit_5h"], &quota); err != nil {
		t.Fatal(err)
	}
	if quota.UsedRatio == nil || *quota.UsedRatio != 0 {
		t.Fatalf("used_ratio=%v, want genuine zero", quota.UsedRatio)
	}
	for _, key := range []string{"limit_7d", "limit_month_total", "limit_month_code"} {
		if string(document.Quotas[key]) != "null" {
			t.Errorf("%s=%s, want null", key, document.Quotas[key])
		}
	}
	if document.Raw["future_provider_field"] != "synthetic-value" {
		t.Fatalf("raw=%+v", document.Raw)
	}
}

func TestWriteJSONReturnsWriterFailure(t *testing.T) {
	writeErr := errors.New("synthetic writer failure")
	snapshot := kimiburn.Snapshot{GeneratedAt: time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC), Raw: json.RawMessage(`{}`)}
	err := kimiburn.WriteJSON(failingWriter{err: writeErr}, snapshot, kimiburn.JSONOptions{})
	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteJSON error=%v, want writer failure", err)
	}
}
