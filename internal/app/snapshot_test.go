package app

import (
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func TestNewSnapshotUsesUnavailableRunwayPlaceholders(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	valid, invalid := 0.25, 1.5
	reset := now.Add(time.Hour)
	reading := domain.UsageReading{
		ObservedAt: now,
		Quotas: map[domain.QuotaKey]domain.QuotaReading{
			domain.QuotaLimit5h:         {Source: domain.SourceRatio, UsedRatio: &valid, ResetAt: &reset},
			domain.QuotaLimit7d:         {Source: domain.SourceRatio, UsedRatio: &valid},
			domain.QuotaLimitMonthCode:  {Source: domain.SourceLegacyCounts, UsedRatio: &invalid, ResetAt: &reset},
			domain.QuotaLimitMonthTotal: {Source: domain.SourceRatio, UsedRatio: &valid, ResetAt: &reset, ConflictReason: domain.ConflictSources},
		},
	}
	snapshot := NewSnapshot(reading)
	if snapshot.GeneratedAt != now || snapshot.SchemaVersion != 1 {
		t.Fatalf("metadata: %+v", snapshot)
	}
	wantReasons := map[domain.QuotaKey]string{
		domain.QuotaLimit5h: "insufficient_samples", domain.QuotaLimit7d: "missing_reset_time",
		domain.QuotaLimitMonthCode: "invalid_ratio", domain.QuotaLimitMonthTotal: "conflicting_sources",
	}
	for key, want := range wantReasons {
		runway := snapshot.Runways[key]
		if runway.Status != "unavailable" || runway.Confidence != "unavailable" || runway.Reason != want || runway.Seconds != nil || runway.SampleCount != 0 {
			t.Errorf("%s runway = %+v, want unavailable/%s", key, runway, want)
		}
	}
	if snapshot.Warnings == nil {
		t.Error("warnings must not be nil")
	}
}
