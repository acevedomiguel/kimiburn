package runway

import (
	"math"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

const (
	minSamples = 5
	minWindow  = 120 * time.Second
)

// Calculate returns a reset-bounded local estimate. It never treats the reset
// countdown as runway and never crosses the quota's reset boundary.
func Calculate(samples []domain.Sample, quota domain.QuotaReading, now time.Time) domain.RunwayEstimate {
	estimate := domain.RunwayEstimate{Status: "unavailable", Confidence: "unavailable"}
	if quota.ConflictReason != "" {
		estimate.Reason = "conflicting_sources"
		return estimate
	}
	if quota.UsedRatio == nil || math.IsNaN(*quota.UsedRatio) || math.IsInf(*quota.UsedRatio, 0) || *quota.UsedRatio < 0 || *quota.UsedRatio > 1 {
		estimate.Reason = "invalid_ratio"
		return estimate
	}
	if *quota.UsedRatio >= 1 {
		zero := 0.0
		estimate.Status, estimate.Seconds = "depleted", &zero
		return estimate
	}
	if quota.ResetAt == nil || quota.ResetAt.IsZero() {
		estimate.Reason = "missing_reset_time"
		return estimate
	}
	estimate.SampleCount = len(samples)
	if len(samples) > 0 {
		estimate.WindowSeconds = int64(samples[len(samples)-1].ObservedAt.Sub(samples[0].ObservedAt).Seconds())
	}
	if len(samples) < minSamples || time.Duration(estimate.WindowSeconds)*time.Second < minWindow {
		estimate.Reason = "insufficient_samples"
		return estimate
	}
	times := make([]float64, len(samples))
	ratios := make([]float64, len(samples))
	var meanT, meanR float64
	for i, sample := range samples {
		times[i] = sample.ObservedAt.Sub(samples[0].ObservedAt).Seconds()
		ratios[i] = sample.UsedRatio
		meanT += times[i]
		meanR += ratios[i]
	}
	meanT /= float64(len(samples))
	meanR /= float64(len(samples))
	var numerator, denominator float64
	for i := range samples {
		dt := times[i] - meanT
		numerator += dt * (ratios[i] - meanR)
		denominator += dt * dt
	}
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		estimate.Reason = "insufficient_samples"
		return estimate
	}
	burn := numerator / denominator
	if math.IsNaN(burn) || math.IsInf(burn, 0) {
		estimate.Reason = "non_finite_estimate"
		return estimate
	}
	estimate.BurnRatioPerSecond = floatPtr(burn)
	percentHour := burn * 3600 * 100
	estimate.BurnPercentPerHour = floatPtr(percentHour)
	estimate.Confidence = confidence(len(samples), time.Duration(estimate.WindowSeconds)*time.Second)
	if burn <= 0 {
		estimate.Status = "no_consumption"
		return estimate
	}
	seconds := (1 - *quota.UsedRatio) / burn
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		estimate.Status = "unavailable"
		estimate.Confidence = "unavailable"
		estimate.Reason = "non_finite_estimate"
		estimate.BurnRatioPerSecond, estimate.BurnPercentPerHour = nil, nil
		return estimate
	}
	estimate.Seconds = floatPtr(seconds)
	observed := now
	if !samples[len(samples)-1].ObservedAt.IsZero() {
		observed = samples[len(samples)-1].ObservedAt
	}
	exhaustion := observed.Add(time.Duration(seconds * float64(time.Second)))
	if exhaustion.Before(*quota.ResetAt) {
		estimate.Status = "finite"
		estimate.ExhaustedAt = timePtr(exhaustion)
	} else {
		estimate.Status = "beyond_reset"
	}
	return estimate
}

// CalculateRunways computes each quota independently from the same reading.
func CalculateRunways(history map[domain.QuotaKey][]domain.Sample, reading domain.UsageReading) map[domain.QuotaKey]domain.RunwayEstimate {
	result := make(map[domain.QuotaKey]domain.RunwayEstimate, len(domain.CanonicalQuotaKeys))
	for _, key := range domain.CanonicalQuotaKeys {
		if quota, ok := reading.Quotas[key]; ok {
			result[key] = Calculate(history[key], quota, reading.ObservedAt)
		}
	}
	return result
}

func floatPtr(v float64) *float64    { return &v }
func timePtr(v time.Time) *time.Time { return &v }

func confidence(count int, window time.Duration) string {
	if count >= 120 && window >= time.Hour {
		return "high"
	}
	if count >= 30 && window >= 15*time.Minute {
		return "medium"
	}
	return "low"
}
