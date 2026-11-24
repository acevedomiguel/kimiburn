package ui

import (
	"encoding/json"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

var syntheticTime = time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)

// SyntheticSnapshot is a deterministic modern-ratio demo. Every value is
// invented; no config, credential, cache, clock, or network is consulted.
func SyntheticSnapshot() domain.Snapshot {
	generated := syntheticTime
	monthReset := generated.Add(18*24*time.Hour + 6*time.Hour)
	fiveReset := generated.Add(3*time.Hour + 12*time.Minute)
	monthUsed, fiveUsed := 0.418, 0.72
	monthSeconds, fiveSeconds := 5600000.0, 8400.0
	monthBurn, fiveBurn := 0.00000010392857142857143, 0.00003333333333333333
	exhausts := generated.Add(8400 * time.Second)
	return domain.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated,
		Quotas: map[domain.QuotaKey]domain.QuotaReading{
			domain.QuotaLimitMonthTotal: {Source: domain.SourceRatio, UsedRatio: &monthUsed, ResetAt: &monthReset, Raw: json.RawMessage(`{"used_ratio":0.418}`)},
			domain.QuotaLimit5h:         {Source: domain.SourceRatio, UsedRatio: &fiveUsed, ResetAt: &fiveReset, Raw: json.RawMessage(`{"used_ratio":0.72}`)},
		},
		Runways: map[domain.QuotaKey]domain.RunwayEstimate{
			domain.QuotaLimitMonthTotal: {Status: "beyond_reset", Seconds: &monthSeconds, BurnRatioPerSecond: &monthBurn, BurnPercentPerHour: floatPointer(monthBurn * 360000), SampleCount: 121, WindowSeconds: 3600, Confidence: "high"},
			domain.QuotaLimit5h:         {Status: "finite", Seconds: &fiveSeconds, ExhaustedAt: &exhausts, BurnRatioPerSecond: &fiveBurn, BurnPercentPerHour: floatPointer(12), SampleCount: 60, WindowSeconds: 1770, Confidence: "medium"},
		},
		GoodsVersion: json.RawMessage(`2`), BoosterWallet: json.RawMessage(`{"synthetic_example_field":null}`),
		Raw:      json.RawMessage(`{"goods_version":2,"usages":{"limit_5h":{"used_ratio":0.72,"reset_time":"2026-11-24T17:44:30Z"},"limit_month_total":{"used_ratio":0.418,"reset_time":"2026-12-12T20:32:30Z"}},"boosterWallet":{"synthetic_example_field":null},"future_provider_field":"synthetic-passthrough-value"}`),
		Warnings: []domain.Warning{},
	}
}

// SyntheticLegacySnapshot adds absolute seven-day request counts to the modern
// demo. It exercises legacy count presentation without I/O.
func SyntheticLegacySnapshot() domain.Snapshot {
	snapshot := SyntheticSnapshot()
	reset := snapshot.GeneratedAt.Add(48 * time.Hour)
	usedRatio, used, limit, remaining, burn := 0.4, 40.0, 100.0, 60.0, 0.0
	snapshot.Quotas[domain.QuotaLimit7d] = domain.QuotaReading{Source: domain.SourceLegacyCounts, UsedRatio: &usedRatio, Used: &used, Limit: &limit, Remaining: &remaining, ResetAt: &reset, Raw: json.RawMessage(`{"limit":100,"detail":40,"remaining":60}`)}
	snapshot.Runways[domain.QuotaLimit7d] = domain.RunwayEstimate{Status: "no_consumption", BurnRatioPerSecond: &burn, BurnPercentPerHour: floatPointer(0), SampleCount: 30, WindowSeconds: 900, Confidence: "medium"}
	snapshot.Raw = json.RawMessage(`{"goods_version":2,"usages":{"limit_5h":{"used_ratio":0.72,"reset_time":"2026-11-24T17:44:30Z"},"limit_month_total":{"used_ratio":0.418,"reset_time":"2026-12-12T20:32:30Z"}},"limits":[{"window":{"duration":10080,"timeUnit":"TIME_UNIT_MINUTE"},"limit":100,"detail":40,"remaining":60}],"boosterWallet":{"synthetic_example_field":null},"future_provider_field":"synthetic-passthrough-value"}`)
	return snapshot
}

// SyntheticUnavailableSnapshot demonstrates a valid response with no canonical
// quotas and a safe parser warning.
func SyntheticUnavailableSnapshot() domain.Snapshot {
	return domain.Snapshot{
		SchemaVersion: 1, GeneratedAt: syntheticTime,
		Quotas: map[domain.QuotaKey]domain.QuotaReading{}, Runways: map[domain.QuotaKey]domain.RunwayEstimate{},
		Raw:      json.RawMessage(`{"future_provider_field":"synthetic-unavailable-value"}`),
		Warnings: []domain.Warning{{Section: "Parser", Message: "No canonical quotas were reported; raw provider data is available with --json."}},
	}
}

func floatPointer(value float64) *float64 { return &value }
