// Package runway derives local burn-rate estimates from observed ratios.
package runway

import (
	"math"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

const (
	MaxSamples = 720
	MaxWindow  = 6 * time.Hour
)

// RecordReading appends only usable live readings. History is independent per
// quota; a reset or ratio decrease invalidates only that quota's period.
func RecordReading(history map[domain.QuotaKey][]domain.Sample, reading domain.UsageReading) {
	if history == nil || reading.ObservedAt.IsZero() {
		return
	}
	for key, quota := range reading.Quotas {
		if quota.UsedRatio == nil || quota.ResetAt == nil || quota.ConflictReason != "" {
			continue
		}
		ratio := *quota.UsedRatio
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 || quota.ResetAt.IsZero() {
			continue
		}
		samples := history[key]
		current := domain.Sample{ObservedAt: reading.ObservedAt.UTC(), UsedRatio: ratio, ResetAt: quota.ResetAt.UTC()}
		if len(samples) > 0 {
			last := samples[len(samples)-1]
			if !last.ResetAt.Equal(current.ResetAt) || ratio < last.UsedRatio {
				samples = nil
			} else if !current.ObservedAt.After(last.ObservedAt) {
				continue
			}
		}
		samples = append(samples, current)
		cutoff := current.ObservedAt.Add(-MaxWindow)
		first := 0
		for first < len(samples)-1 && samples[first].ObservedAt.Before(cutoff) {
			first++
		}
		if excess := len(samples) - first - MaxSamples; excess > 0 {
			first += excess
		}
		if first > 0 {
			samples = append([]domain.Sample(nil), samples[first:]...)
		}
		history[key] = samples
	}
}
