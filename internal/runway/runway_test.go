package runway

import (
	"math"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

var (
	testStart = time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
	testReset = testStart.Add(24 * time.Hour)
)

func sampleSeries(count int, spacing time.Duration, first, step float64) []domain.Sample {
	samples := make([]domain.Sample, count)
	for i := range samples {
		samples[i] = domain.Sample{ObservedAt: testStart.Add(time.Duration(i) * spacing), UsedRatio: first + float64(i)*step, ResetAt: testReset}
	}
	return samples
}

func TestCalculateInsufficientAndUnavailableReasons(t *testing.T) {
	invalid := math.NaN()
	for _, test := range []struct {
		name, reason string
		quota        domain.QuotaReading
		samples      []domain.Sample
	}{
		{"fewSamples", "insufficient_samples", domain.QuotaReading{UsedRatio: floatPointer(.5), ResetAt: timePointer(testReset)}, sampleSeries(4, 30*time.Second, .2, .01)},
		{"shortWindow", "insufficient_samples", domain.QuotaReading{UsedRatio: floatPointer(.5), ResetAt: timePointer(testReset)}, sampleSeries(5, time.Second, .2, .01)},
		{"flatTimes", "insufficient_samples", domain.QuotaReading{UsedRatio: floatPointer(.5), ResetAt: timePointer(testReset)}, []domain.Sample{{ObservedAt: testStart, UsedRatio: .2, ResetAt: testReset}, {ObservedAt: testStart, UsedRatio: .3, ResetAt: testReset}, {ObservedAt: testStart, UsedRatio: .4, ResetAt: testReset}, {ObservedAt: testStart, UsedRatio: .5, ResetAt: testReset}, {ObservedAt: testStart, UsedRatio: .6, ResetAt: testReset}}},
		{"missingReset", "missing_reset_time", domain.QuotaReading{UsedRatio: floatPointer(.5)}, nil},
		{"invalid", "invalid_ratio", domain.QuotaReading{UsedRatio: &invalid, ResetAt: timePointer(testReset)}, nil},
		{"conflict", "conflicting_sources", domain.QuotaReading{ConflictReason: "ratio_mismatch", UsedRatio: floatPointer(.5), ResetAt: timePointer(testReset)}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Calculate(test.samples, test.quota, testStart)
			if got.Status != "unavailable" || got.Reason != test.reason {
				t.Fatalf("estimate=%+v", got)
			}
			if got.Seconds != nil || got.ExhaustedAt != nil || got.BurnRatioPerSecond != nil {
				t.Fatalf("unavailable values must be null: %+v", got)
			}
		})
	}
}

func TestCalculateRegressionAndResetBoundaries(t *testing.T) {
	samples := sampleSeries(5, 30*time.Second, .2, .01) // burn = .01/30 = 1/3000 per second.
	for _, test := range []struct {
		name, status string
		ratio        float64
		seconds      float64
		resetAfter   time.Duration
	}{
		{"finite", "finite", .4, 1800, time.Hour},
		{"exactReset", "beyond_reset", .6, 1200, 1200 * time.Second},
		{"pastReset", "beyond_reset", .61, 1170, 1169 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			reset := testStart.Add(120 * time.Second).Add(test.resetAfter)
			got := Calculate(samples, domain.QuotaReading{UsedRatio: floatPointer(test.ratio), ResetAt: &reset}, testStart.Add(120*time.Second))
			if got.Status != test.status || got.Seconds == nil || math.Abs(*got.Seconds-test.seconds) > 1e-8 {
				t.Fatalf("estimate=%+v", got)
			}
			if got.BurnRatioPerSecond == nil || math.Abs(*got.BurnRatioPerSecond-1.0/3000) > 1e-12 {
				t.Fatalf("burn=%v", got.BurnRatioPerSecond)
			}
			if test.status == "finite" {
				if got.ExhaustedAt == nil || !got.ExhaustedAt.Before(reset) {
					t.Fatalf("timestamp=%v reset=%v", got.ExhaustedAt, reset)
				}
			} else if got.ExhaustedAt != nil {
				t.Fatalf("reset boundary must not have timestamp: %v", got.ExhaustedAt)
			}
		})
	}
}

func TestCalculateDepletedNoConsumptionAndNonFinite(t *testing.T) {
	one := 1.0
	got := Calculate(nil, domain.QuotaReading{UsedRatio: &one, ResetAt: timePointer(testReset)}, testStart)
	if got.Status != "depleted" || got.Seconds == nil || *got.Seconds != 0 || got.ExhaustedAt != nil || got.Confidence != "unavailable" {
		t.Fatalf("depleted=%+v", got)
	}
	got = Calculate(sampleSeries(5, 30*time.Second, .5, 0), domain.QuotaReading{UsedRatio: floatPointer(.5), ResetAt: timePointer(testReset)}, testStart.Add(120*time.Second))
	if got.Status != "no_consumption" || got.Seconds != nil || got.ExhaustedAt != nil || got.BurnRatioPerSecond == nil || *got.BurnRatioPerSecond != 0 {
		t.Fatalf("flat=%+v", got)
	}
	inf := math.Inf(1)
	got = Calculate(sampleSeries(5, 30*time.Second, .2, .01), domain.QuotaReading{UsedRatio: &inf, ResetAt: timePointer(testReset)}, testStart)
	if got.Status != "unavailable" || got.Reason != "invalid_ratio" {
		t.Fatalf("infinite=%+v", got)
	}
}

func TestConfidenceThresholds(t *testing.T) {
	for _, test := range []struct {
		name    string
		samples []domain.Sample
		at      time.Time
		want    string
	}{
		{"minimumLow", sampleSeries(5, 30*time.Second, .2, .0001), testStart.Add(120 * time.Second), "low"},
		{"countOnlyLow", sampleSeries(30, 4*time.Second, .2, .0001), testStart.Add(116 * time.Second), "insufficient_samples"},
		{"medium", sampleSeries(30, 32*time.Second, .2, .0001), testStart.Add(928 * time.Second), "medium"},
		{"high", sampleSeries(120, 31*time.Second, .2, .0001), testStart.Add(3689 * time.Second), "high"},
		{"longLow", sampleSeries(5, 30*time.Minute, .2, .001), testStart.Add(120 * time.Minute), "low"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Calculate(test.samples, domain.QuotaReading{UsedRatio: floatPointer(.9999), ResetAt: timePointer(test.at.Add(24 * time.Hour))}, test.at)
			if test.want == "insufficient_samples" {
				if got.Reason != test.want {
					t.Fatalf("estimate=%+v", got)
				}
			} else if got.Confidence != test.want || got.Status != "finite" {
				t.Fatalf("estimate=%+v", got)
			}
		})
	}
}

func TestNoForecastProducesNaNOrInf(t *testing.T) {
	samples := sampleSeries(5, 30*time.Second, .2, .01)
	for _, ratio := range []float64{0, .2, .5, .6, 1} {
		got := Calculate(samples, domain.QuotaReading{UsedRatio: floatPointer(ratio), ResetAt: timePointer(testReset)}, testStart)
		for name, value := range map[string]*float64{"seconds": got.Seconds, "burn": got.BurnRatioPerSecond} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				t.Fatalf("ratio=%v %s=%v", ratio, name, *value)
			}
		}
		if got.ExhaustedAt != nil && !got.ExhaustedAt.Before(testReset) {
			t.Fatalf("timestamp=%v reset=%v", got.ExhaustedAt, testReset)
		}
	}
}

func TestCalculateRunwaysUsesCurrentWindowAverageOnFirstRead(t *testing.T) {
	for _, test := range []struct {
		key       domain.QuotaKey
		now       time.Time
		reset     time.Time
		used      float64
		wantStart time.Time
	}{
		{
			key: domain.QuotaLimit5h, now: testStart,
			reset: testStart.Add(2*time.Hour + 42*time.Minute), used: .209,
			wantStart: testStart.Add(-2*time.Hour - 18*time.Minute),
		},
		{
			key:   domain.QuotaLimitMonthTotal,
			now:   time.Date(2026, 11, 24, 9, 24, 53, 0, time.UTC),
			reset: time.Date(2026, 12, 17, 0, 0, 0, 0, time.UTC), used: .2204,
			wantStart: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			key:   domain.QuotaLimitMonthCode,
			now:   time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			reset: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), used: .5,
			wantStart: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(string(test.key), func(t *testing.T) {
			reading := domain.UsageReading{ObservedAt: test.now, Quotas: map[domain.QuotaKey]domain.QuotaReading{
				test.key: {UsedRatio: floatPointer(test.used), ResetAt: timePointer(test.reset)},
			}}
			history := map[domain.QuotaKey][]domain.Sample{test.key: {{ObservedAt: test.now, UsedRatio: test.used, ResetAt: test.reset}}}
			got := CalculateRunways(history, reading)[test.key]
			wantBurn := test.used / test.now.Sub(test.wantStart).Seconds()
			wantSeconds := (1 - test.used) / wantBurn
			if got.Status != "beyond_reset" || got.Basis != "window_average" {
				t.Fatalf("estimate=%+v", got)
			}
			if got.BurnRatioPerSecond == nil || math.Abs(*got.BurnRatioPerSecond-wantBurn) > 1e-12 {
				t.Fatalf("burn=%v want=%v", got.BurnRatioPerSecond, wantBurn)
			}
			if got.Seconds == nil || math.Abs(*got.Seconds-wantSeconds) > 1e-6 || got.ExhaustedAt != nil {
				t.Fatalf("seconds=%v timestamp=%v want=%v", got.Seconds, got.ExhaustedAt, wantSeconds)
			}
		})
	}
}

func TestCalculateRunwaysMonthlyAlwaysUsesCurrentWindowAverage(t *testing.T) {
	now := time.Date(2026, 11, 24, 9, 24, 53, 0, time.UTC)
	reset := time.Date(2026, 12, 17, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		key        domain.QuotaKey
		used, step float64
		wantStatus string
	}{
		{name: "membership", key: domain.QuotaLimitMonthTotal, used: .5, step: .05, wantStatus: "finite"},
		{name: "codeAllocation", key: domain.QuotaLimitMonthCode, used: .5, step: .05, wantStatus: "finite"},
		{name: "zeroUsage", key: domain.QuotaLimitMonthTotal, used: 0, step: 0, wantStatus: "no_consumption"},
	} {
		t.Run(test.name, func(t *testing.T) {
			samples := make([]domain.Sample, 5)
			for i := range samples {
				samples[i] = domain.Sample{
					ObservedAt: now.Add(time.Duration(i-4) * 30 * time.Second),
					UsedRatio:  test.used - float64(4-i)*test.step,
					ResetAt:    reset,
				}
			}
			reading := domain.UsageReading{ObservedAt: now, Quotas: map[domain.QuotaKey]domain.QuotaReading{
				test.key: {UsedRatio: floatPointer(test.used), ResetAt: timePointer(reset)},
			}}
			got := CalculateRunways(map[domain.QuotaKey][]domain.Sample{test.key: samples}, reading)[test.key]
			wantBurn := test.used / now.Sub(start).Seconds()
			if got.Basis != "window_average" || got.Status != test.wantStatus {
				t.Fatalf("estimate=%+v", got)
			}
			if got.BurnRatioPerSecond == nil || math.Abs(*got.BurnRatioPerSecond-wantBurn) > 1e-12 {
				t.Fatalf("burn=%v want=%v", got.BurnRatioPerSecond, wantBurn)
			}
			localBurn := test.step / (30 * time.Second).Seconds()
			if test.step > 0 && math.Abs(*got.BurnRatioPerSecond-localBurn) <= 1e-12 {
				t.Fatalf("monthly burn used local sample slope: %v", *got.BurnRatioPerSecond)
			}
			if test.wantStatus == "no_consumption" {
				if got.Seconds != nil || got.ExhaustedAt != nil {
					t.Fatalf("zero-consumption forecast=%+v", got)
				}
				return
			}
			wantSeconds := (1 - test.used) / wantBurn
			if got.Seconds == nil || math.Abs(*got.Seconds-wantSeconds) > 1e-6 {
				t.Fatalf("seconds=%v want=%v", got.Seconds, wantSeconds)
			}
			if got.ExhaustedAt == nil || !got.ExhaustedAt.Before(reset) {
				t.Fatalf("timestamp=%v reset=%v", got.ExhaustedAt, reset)
			}
		})
	}
}

func floatPointer(v float64) *float64    { return &v }
func timePointer(v time.Time) *time.Time { return &v }
