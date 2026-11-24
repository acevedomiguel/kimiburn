package app

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func TestWriteJSONGolden(t *testing.T) {
	generated := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	reset5h := generated.Add(11520 * time.Second)
	resetMonth := generated.Add(1576800 * time.Second)
	used5h, usedMonth := 0.72, 0.418
	snapshot := domain.Snapshot{
		SchemaVersion: domain.SnapshotSchemaVersion,
		GeneratedAt:   generated,
		Quotas: map[domain.QuotaKey]domain.QuotaReading{
			domain.QuotaLimit5h:         {Source: domain.SourceRatio, UsedRatio: &used5h, ResetAt: &reset5h, Raw: json.RawMessage(`{"used_ratio":0.72}`)},
			domain.QuotaLimitMonthTotal: {Source: domain.SourceRatio, UsedRatio: &usedMonth, ResetAt: &resetMonth, Raw: json.RawMessage(`{"used_ratio":0.418}`)},
		},
		Runways: map[domain.QuotaKey]domain.RunwayEstimate{
			domain.QuotaLimit5h:         {Status: "unavailable", Reason: "insufficient_samples", Confidence: "unavailable"},
			domain.QuotaLimitMonthTotal: {Status: "unavailable", Reason: "insufficient_samples", Confidence: "unavailable"},
		},
		GoodsVersion:  json.RawMessage(`2`),
		BoosterWallet: json.RawMessage(`{"synthetic_example_field":null}`),
		Raw:           json.RawMessage(`{"goods_version":2,"usages":{"limit_5h":{"used_ratio":0.72,"reset_time":"2026-11-24T17:44:30Z"},"limit_month_total":{"used_ratio":0.418,"reset_time":"2026-12-12T20:32:30Z"}},"boosterWallet":{"synthetic_example_field":null},"future_provider_field":"synthetic-passthrough-value"}`),
		Warnings:      []domain.Warning{},
	}

	var out bytes.Buffer
	if err := WriteJSON(&out, snapshot, ProjectionOptions{APIBase: "https://api.kimi.com/coding/v1", Now: generated.Add(20 * time.Second)}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	want, err := os.ReadFile("testdata/live.golden")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want) {
		t.Fatalf("JSON mismatch\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
}

func TestWriteJSONCachedAgeAndResetSemantics(t *testing.T) {
	generated := time.Date(2026, 11, 24, 14, 32, 30, 123456789, time.FixedZone("test", 3600))
	reset := generated.Add(-time.Second)
	used := 0.0
	snapshot := domain.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated,
		Quotas:  map[domain.QuotaKey]domain.QuotaReading{domain.QuotaLimit5h: {Source: domain.SourceRatio, UsedRatio: &used, ResetAt: &reset}},
		Runways: map[domain.QuotaKey]domain.RunwayEstimate{domain.QuotaLimit5h: {Status: "unavailable", Reason: "insufficient_samples", Confidence: "unavailable"}},
		Raw:     json.RawMessage(`{}`),
	}
	var out bytes.Buffer
	if err := WriteJSON(&out, snapshot, ProjectionOptions{APIBase: "https://api.kimi.ai/coding/v1", Cached: true, Now: generated.Add(65 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		GeneratedAt string           `json:"generated_at"`
		Cached      bool             `json:"cached"`
		CacheAge    int64            `json:"cache_age_seconds"`
		Warnings    []domain.Warning `json:"warnings"`
		Quotas      map[string]struct {
			ResetAt string `json:"reset_at"`
			ResetIn *int64 `json:"reset_in_seconds"`
			Runway  struct {
				Reason *string `json:"reason"`
			} `json:"runway"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Cached || decoded.CacheAge != 65 {
		t.Fatalf("cached=%v age=%d, want true/65", decoded.Cached, decoded.CacheAge)
	}
	if decoded.GeneratedAt != "2026-11-24T13:32:30.123456789Z" {
		t.Errorf("generated_at=%q", decoded.GeneratedAt)
	}
	if decoded.Warnings == nil {
		t.Error("warnings must be [] not null")
	}
	quota := decoded.Quotas["limit_5h"]
	if quota.ResetAt != "2026-11-24T13:32:29.123456789Z" || quota.ResetIn == nil || *quota.ResetIn != 0 {
		t.Errorf("reset semantics: %+v", quota)
	}
	if quota.Runway.Reason == nil || *quota.Runway.Reason != "insufficient_samples" {
		t.Errorf("reason=%v", quota.Runway.Reason)
	}
}
