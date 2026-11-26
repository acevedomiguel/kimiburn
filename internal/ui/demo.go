package ui

import (
	"encoding/json"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

var syntheticTime = time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)

// SyntheticSnapshot is a deterministic modern-ratio demo. Every value is
// invented; no config, credential, cache, clock, or network is consulted.
func SyntheticSnapshot() kimi.Snapshot {
	generated := syntheticTime
	monthReset := generated.Add(18*24*time.Hour + 6*time.Hour)
	fiveReset := generated.Add(3*time.Hour + 12*time.Minute)
	monthUsed, fiveUsed := 0.418, 0.72
	monthSeconds, fiveSeconds := 5600000.0, 8400.0
	monthBurn, fiveBurn := 0.00000010392857142857143, 0.00003333333333333333
	exhausts := generated.Add(8400 * time.Second)
	return kimi.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated,
		Quotas: map[kimi.QuotaKey]kimi.QuotaReading{
			kimi.QuotaLimitMonthTotal: {Source: kimi.SourceRatio, UsedRatio: &monthUsed, ResetAt: &monthReset, Raw: json.RawMessage(`{"used_ratio":0.418}`)},
			kimi.QuotaLimit5h:         {Source: kimi.SourceRatio, UsedRatio: &fiveUsed, ResetAt: &fiveReset, Raw: json.RawMessage(`{"used_ratio":0.72}`)},
		},
		Runways: map[kimi.QuotaKey]kimi.RunwayEstimate{
			kimi.QuotaLimitMonthTotal: {Status: "beyond_reset", Seconds: &monthSeconds, BurnRatioPerSecond: &monthBurn, BurnPercentPerHour: floatPointer(monthBurn * 360000), SampleCount: 121, WindowSeconds: 3600, Confidence: "high"},
			kimi.QuotaLimit5h:         {Status: "finite", Seconds: &fiveSeconds, ExhaustedAt: &exhausts, BurnRatioPerSecond: &fiveBurn, BurnPercentPerHour: floatPointer(12), SampleCount: 60, WindowSeconds: 1770, Confidence: "medium"},
		},
		GoodsVersion: json.RawMessage(`2`), BoosterWallet: json.RawMessage(`{"synthetic_example_field":null}`),
		Raw:      json.RawMessage(`{"goods_version":2,"usages":{"limit_5h":{"used_ratio":0.72,"reset_time":"2026-11-24T17:44:30Z"},"limit_month_total":{"used_ratio":0.418,"reset_time":"2026-12-12T20:32:30Z"}},"boosterWallet":{"synthetic_example_field":null},"future_provider_field":"synthetic-passthrough-value"}`),
		Warnings: []kimi.Warning{},
	}
}

// SyntheticLegacySnapshot adds the Monthly Code allocation and absolute
// seven-day request counts to the modern demo. It exercises the Code allocation
// and legacy count panels without I/O, and it is the fixture behind
// `kimiburn demo`.
func SyntheticLegacySnapshot() kimi.Snapshot {
	snapshot := SyntheticSnapshot()
	reset := snapshot.GeneratedAt.Add(48 * time.Hour)
	usedRatio, used, limit, remaining, burn := 0.4, 40.0, 100.0, 60.0, 0.0
	snapshot.Quotas[kimi.QuotaLimit7d] = kimi.QuotaReading{Source: kimi.SourceLegacyCounts, UsedRatio: &usedRatio, Used: &used, Limit: &limit, Remaining: &remaining, ResetAt: &reset, Raw: json.RawMessage(`{"limit":100,"detail":40,"remaining":60}`)}
	snapshot.Runways[kimi.QuotaLimit7d] = kimi.RunwayEstimate{Status: "no_consumption", BurnRatioPerSecond: &burn, BurnPercentPerHour: floatPointer(0), SampleCount: 30, WindowSeconds: 900, Confidence: "medium"}

	// The Code allocation is part of the shared membership, so it resets with the
	// monthly quota and is never added to it.
	codeUsed := 0.2
	codeReset := snapshot.GeneratedAt.Add(18*24*time.Hour + 6*time.Hour)
	snapshot.Quotas[kimi.QuotaLimitMonthCode] = kimi.QuotaReading{Source: kimi.SourceRatio, UsedRatio: &codeUsed, ResetAt: &codeReset, Raw: json.RawMessage(`{"used_ratio":0.2}`)}
	snapshot.Raw = json.RawMessage(`{"goods_version":2,"usages":{"limit_5h":{"used_ratio":0.72,"reset_time":"2026-11-24T17:44:30Z"},"limit_month_total":{"used_ratio":0.418,"reset_time":"2026-12-12T20:32:30Z"}},"limits":[{"window":{"duration":10080,"timeUnit":"TIME_UNIT_MINUTE"},"limit":100,"detail":40,"remaining":60}],"boosterWallet":{"synthetic_example_field":null},"future_provider_field":"synthetic-passthrough-value"}`)
	return snapshot
}

// SyntheticUnavailableSnapshot demonstrates a valid response with no canonical
// quotas and a safe parser warning.
func SyntheticUnavailableSnapshot() kimi.Snapshot {
	return kimi.Snapshot{
		SchemaVersion: 1, GeneratedAt: syntheticTime,
		Quotas: map[kimi.QuotaKey]kimi.QuotaReading{}, Runways: map[kimi.QuotaKey]kimi.RunwayEstimate{},
		Raw:      json.RawMessage(`{"future_provider_field":"synthetic-unavailable-value"}`),
		Warnings: []kimi.Warning{{Section: "Parser", Message: "No canonical quotas were reported; raw provider data is available with --json."}},
	}
}

func floatPointer(value float64) *float64 { return &value }

// SyntheticTime is the instant the synthetic snapshots were "read". The demo
// clock starts here so reset countdowns tick from the README values.
func SyntheticTime() time.Time { return syntheticTime }

// SyntheticHistory is an invented sample history consistent with the demo's
// runway figures: the 5-hour quota climbs about 5.9% over 29.5 minutes in
// uneven bursts, the monthly membership barely moves, and the legacy seven-day
// quota is flat.
func SyntheticHistory() map[kimi.QuotaKey][]kimi.Sample {
	snapshot := SyntheticLegacySnapshot()
	history := make(map[kimi.QuotaKey][]kimi.Sample)

	// Burst weights repeat every 12 samples; the cumulative curve is scaled so
	// the last sample lands exactly on the snapshot's used ratio.
	weights := []float64{0, 1, 3, 2, 0, 0, 1, 4, 5, 2, 1, 0}
	series := func(key kimi.QuotaKey, count int, window time.Duration, rise float64) {
		quota := snapshot.Quotas[key]
		end := *quota.UsedRatio
		cumulative := make([]float64, count)
		total := 0.0
		for i := 1; i < count; i++ {
			total += weights[i%len(weights)]
			cumulative[i] = total
		}
		step := window / time.Duration(count-1)
		samples := make([]kimi.Sample, count)
		for i := range samples {
			progress := 0.0
			if total > 0 {
				progress = cumulative[i] / total
			}
			samples[i] = kimi.Sample{
				ObservedAt: syntheticTime.Add(-window + time.Duration(i)*step),
				UsedRatio:  end - rise + rise*progress,
				ResetAt:    quota.ResetAt.UTC(),
			}
		}
		history[key] = samples
	}
	series(kimi.QuotaLimit5h, 60, 1770*time.Second, 0.059)
	series(kimi.QuotaLimitMonthTotal, 121, time.Hour, 0.0004)
	series(kimi.QuotaLimit7d, 30, 900*time.Second, 0)
	return history
}
