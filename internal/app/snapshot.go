// Package app builds provider-neutral snapshots and stable public projections.
package app

import (
	"encoding/json"
	"math"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// NewSnapshot converts one successful live reading into the internal snapshot
// shape. History estimation is layered on later; the first read always uses
// honest unavailable runway placeholders.
func NewSnapshot(reading domain.UsageReading) domain.Snapshot {
	warnings := append([]domain.Warning{}, reading.Warnings...)
	snapshot := domain.Snapshot{
		SchemaVersion: domain.SnapshotSchemaVersion,
		GeneratedAt:   reading.ObservedAt.UTC(),
		Quotas:        reading.Quotas,
		Runways:       make(map[domain.QuotaKey]domain.RunwayEstimate, len(reading.Quotas)),
		GoodsVersion:  cloneRaw(reading.GoodsVersion),
		BoosterWallet: cloneRaw(reading.BoosterWallet),
		Raw:           cloneRaw(reading.Raw),
		Warnings:      warnings,
	}
	for key, quota := range reading.Quotas {
		reason := "insufficient_samples"
		switch {
		case quota.ConflictReason != "":
			reason = "conflicting_sources"
		case quota.UsedRatio == nil || math.IsNaN(*quota.UsedRatio) || math.IsInf(*quota.UsedRatio, 0) || *quota.UsedRatio < 0 || *quota.UsedRatio > 1:
			reason = "invalid_ratio"
		case quota.ResetAt == nil:
			reason = "missing_reset_time"
		}
		snapshot.Runways[key] = domain.RunwayEstimate{
			Status: "unavailable", Reason: reason, Confidence: "unavailable",
		}
	}
	return snapshot
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage{}, raw...)
}
