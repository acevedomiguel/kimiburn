package kimi

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func decodeSynthetic(t *testing.T, raw string) domain.UsageReading {
	t.Helper()
	return decodeUsage(json.RawMessage(raw), fixedNow)
}

func requireRatio(t *testing.T, quota domain.QuotaReading, want float64) {
	t.Helper()
	if quota.UsedRatio == nil || *quota.UsedRatio != want {
		t.Fatalf("used ratio = %v, want %v", quota.UsedRatio, want)
	}
}

func requireCount(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("count = %v, want %v", got, want)
	}
}

func requireNoWarnings(t *testing.T, reading domain.UsageReading) {
	t.Helper()
	if len(reading.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", reading.Warnings)
	}
}

func TestDecodeModernEveryQuotaAndRawPassthrough(t *testing.T) {
	raw := `{"usages":{` +
		`"limit_5h":{"used_ratio":0,"reset_time":"2026-11-24T17:00:00Z","future":"keep"},` +
		`"limit_7d":{"used_ratio":"0.25","reset_time":"2026-11-25T12:00:00.123456789+02:00"},` +
		`"limit_month_total":{"used_ratio":1},` +
		`"limit_month_code":{"used_ratio":0.5}},` +
		`"goods_version":{"synthetic":2},"boosterWallet":{"synthetic":null},"unknown":[1,{"x":true}]}`
	reading := decodeSynthetic(t, raw)
	requireNoWarnings(t, reading)
	if string(reading.Raw) != raw || string(reading.GoodsVersion) != `{"synthetic":2}` || string(reading.BoosterWallet) != `{"synthetic":null}` {
		t.Fatalf("raw passthrough changed: raw=%s goods=%s wallet=%s", reading.Raw, reading.GoodsVersion, reading.BoosterWallet)
	}
	if !reading.ObservedAt.Equal(fixedNow) {
		t.Fatalf("observed at = %v, want %v", reading.ObservedAt, fixedNow)
	}

	wantRatios := map[domain.QuotaKey]float64{
		domain.QuotaLimit5h:         0,
		domain.QuotaLimit7d:         0.25,
		domain.QuotaLimitMonthTotal: 1,
		domain.QuotaLimitMonthCode:  0.5,
	}
	for key, want := range wantRatios {
		quota, ok := reading.Quotas[key]
		if !ok {
			t.Fatalf("quota %q missing", key)
		}
		requireRatio(t, quota, want)
		if quota.Source != domain.SourceRatio {
			t.Errorf("quota %q source = %q, want ratio", key, quota.Source)
		}
	}
	if got := reading.Quotas[domain.QuotaLimit5h].ResetAt; got == nil || got.Format(time.RFC3339Nano) != "2026-11-24T17:00:00Z" {
		t.Errorf("RFC3339 reset = %v", got)
	}
	if got := reading.Quotas[domain.QuotaLimit7d].ResetAt; got == nil || got.Format(time.RFC3339Nano) != "2026-11-25T12:00:00.123456789+02:00" {
		t.Errorf("RFC3339Nano reset = %v", got)
	}
	if got := string(reading.Quotas[domain.QuotaLimit5h].Raw); got != `{"used_ratio":0,"reset_time":"2026-11-24T17:00:00Z","future":"keep"}` {
		t.Errorf("entry raw = %s", got)
	}
	if *reading.Quotas[domain.QuotaLimitMonthTotal].UsedRatio+*reading.Quotas[domain.QuotaLimitMonthCode].UsedRatio == *reading.Quotas[domain.QuotaLimitMonthTotal].UsedRatio {
		t.Fatal("monthly Code allocation must remain independent")
	}
}

func TestDecodeModernMissingNullAndZeroRemainDistinct(t *testing.T) {
	reading := decodeSynthetic(t, `{"usages":{"limit_5h":{"used_ratio":0},"limit_7d":{"used_ratio":null},"limit_month_total":{}}}`)
	requireNoWarnings(t, reading)
	zero, ok := reading.Quotas[domain.QuotaLimit5h]
	if !ok {
		t.Fatal("genuine zero quota missing")
	}
	requireRatio(t, zero, 0)
	for _, key := range []domain.QuotaKey{domain.QuotaLimit7d, domain.QuotaLimitMonthTotal, domain.QuotaLimitMonthCode} {
		if _, ok := reading.Quotas[key]; ok {
			t.Errorf("quota %q with no usable ratio must be unreported", key)
		}
	}
	empty := decodeSynthetic(t, `{}`)
	if len(empty.Quotas) != 0 || len(empty.Warnings) != 0 {
		t.Fatalf("valid empty object = %+v, want an empty successful reading", empty)
	}
}

func TestDecodeModernRejectsMalformedNonfiniteAndOutOfRangeRatios(t *testing.T) {
	values := []string{`"not-a-number"`, `"NaN"`, `"+Inf"`, `1e999`, `-0.01`, `1.01`, `true`, `[]`, `{}`}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			reading := decodeSynthetic(t, `{"usages":{"limit_5h":{"used_ratio":`+value+`},"limit_7d":{"used_ratio":0.5}}}`)
			bad, ok := reading.Quotas[domain.QuotaLimit5h]
			if !ok || bad.UsedRatio != nil {
				t.Fatalf("invalid modern quota = %+v, want present with unavailable ratio", bad)
			}
			requireRatio(t, reading.Quotas[domain.QuotaLimit7d], 0.5)
			if len(reading.Warnings) != 1 || reading.Warnings[0].Section != string(domain.QuotaLimit5h) {
				t.Fatalf("warnings = %v, want one deterministic limit_5h warning", reading.Warnings)
			}
		})
	}
}

func TestDecodeModernWarnsForInvalidResetWithoutRatio(t *testing.T) {
	for _, entry := range []string{`{"reset_time":123}`, `{"used_ratio":null,"reset_time":123}`} {
		reading := decodeSynthetic(t, `{"usages":{"limit_5h":`+entry+`}}`)
		if _, ok := reading.Quotas[domain.QuotaLimit5h]; ok {
			t.Fatalf("entry %s produced a quota without a ratio", entry)
		}
		if len(reading.Warnings) != 1 {
			t.Fatalf("entry %s warnings = %v, want one", entry, reading.Warnings)
		}
	}
}

func TestDecodeModernResetTimeStrictnessDoesNotBlockRatio(t *testing.T) {
	valid := decodeSynthetic(t, `{"usages":{"limit_5h":{"used_ratio":0.2,"reset_time":"2026-11-24T17:44:30.123456789Z"},"limit_7d":{"used_ratio":0.3,"reset_time":null}}}`)
	requireNoWarnings(t, valid)
	if valid.Quotas[domain.QuotaLimit5h].ResetAt == nil || valid.Quotas[domain.QuotaLimit7d].ResetAt != nil {
		t.Fatalf("valid/null reset decoding = %+v", valid.Quotas)
	}
	for _, value := range []string{`"2026-11-24 17:44:30"`, `"2026-11-24T17:44:30"`, `1764000000`, `true`} {
		t.Run(value, func(t *testing.T) {
			reading := decodeSynthetic(t, `{"usages":{"limit_5h":{"used_ratio":0.2,"reset_time":`+value+`},"limit_7d":{"used_ratio":0.3}}}`)
			quota := reading.Quotas[domain.QuotaLimit5h]
			requireRatio(t, quota, 0.2)
			if quota.ResetAt != nil {
				t.Fatalf("unsupported reset normalized to %v", quota.ResetAt)
			}
			requireRatio(t, reading.Quotas[domain.QuotaLimit7d], 0.3)
			if len(reading.Warnings) != 1 {
				t.Fatalf("warnings = %v, want one", reading.Warnings)
			}
		})
	}
}

func TestDecodeLegacyWeeklyDirectAndDerivedCounts(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"direct", `{"usage":{"limit":"100","used":25,"remaining":"75","resetTime":"2026-11-25T12:00:00.123456789Z"}}`},
		{"derived", `{"usage":{"limit":100,"remaining":75}}`},
		{"null used derives", `{"usage":{"limit":100,"used":null,"remaining":75}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reading := decodeSynthetic(t, tc.body)
			requireNoWarnings(t, reading)
			quota := reading.Quotas[domain.QuotaLimit7d]
			if quota.Source != domain.SourceLegacyCounts {
				t.Fatalf("source = %q, want legacy_counts", quota.Source)
			}
			requireRatio(t, quota, 0.25)
			requireCount(t, quota.Limit, 100)
			requireCount(t, quota.Used, 25)
			requireCount(t, quota.Remaining, 75)
		})
	}
}

func TestDecodeLegacyFiveHourWindowFilterIsExact(t *testing.T) {
	reading := decodeSynthetic(t, `{"limits":[`+
		`{"duration":300,"timeUnit":"TIME_UNIT_MINUTE","limit":10,"used":2,"remaining":8,"future":"keep"},`+
		`{"duration":301,"timeUnit":"TIME_UNIT_MINUTE","limit":20,"used":4},`+
		`{"duration":300,"timeUnit":"TIME_UNIT_HOUR","limit":30,"used":6},`+
		`{"duration":"300","timeUnit":"TIME_UNIT_MINUTE","limit":40,"used":8}]}`)
	requireNoWarnings(t, reading)
	quota := reading.Quotas[domain.QuotaLimit5h]
	requireRatio(t, quota, 0.2)
	requireCount(t, quota.Limit, 10)
	if len(reading.Quotas) != 1 {
		t.Fatalf("quotas = %+v, want only exact 300-minute window", reading.Quotas)
	}
}

func TestDecodeLegacyZeroAndInvalidCounts(t *testing.T) {
	t.Run("zero limit and zero use", func(t *testing.T) {
		reading := decodeSynthetic(t, `{"usage":{"limit":0,"used":0,"remaining":0}}`)
		requireNoWarnings(t, reading)
		quota := reading.Quotas[domain.QuotaLimit7d]
		if quota.UsedRatio != nil {
			t.Fatalf("zero-limit ratio = %v, want unavailable", *quota.UsedRatio)
		}
		requireCount(t, quota.Limit, 0)
		requireCount(t, quota.Used, 0)
	})

	cases := []struct {
		name string
		body string
	}{
		{"zero limit positive use", `{"usage":{"limit":0,"used":1,"remaining":0}}`},
		{"negative limit", `{"usage":{"limit":-1,"used":0}}`},
		{"negative used", `{"usage":{"limit":1,"used":-1}}`},
		{"negative remaining", `{"usage":{"limit":1,"used":0,"remaining":-1}}`},
		{"nonfinite limit", `{"usage":{"limit":"Inf","used":0}}`},
		{"nonfinite used", `{"usage":{"limit":1,"used":"NaN"}}`},
		{"nonfinite remaining", `{"usage":{"limit":1,"remaining":"-Inf"}}`},
		{"use above limit", `{"usage":{"limit":1,"used":2,"remaining":0}}`},
		{"derived negative use", `{"usage":{"limit":1,"remaining":2}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reading := decodeSynthetic(t, tc.body)
			quota := reading.Quotas[domain.QuotaLimit7d]
			if quota.UsedRatio != nil {
				t.Fatalf("invalid legacy ratio = %v, want unavailable", *quota.UsedRatio)
			}
			if len(reading.Warnings) != 1 {
				t.Fatalf("warnings = %v, want one deterministic warning", reading.Warnings)
			}
		})
	}
}

func TestDecodeModernLegacyRatioTolerance(t *testing.T) {
	cases := []struct {
		name        string
		modern      string
		legacyLimit string
		legacyUsed  string
		conflict    bool
	}{
		{"inside tolerance", "0.009", "1000", "5", false},
		{"at tolerance", "0.010", "1000", "5", false},
		{"base shifted at tolerance", "0.505", "1000", "500", false},
		{"outside tolerance", "0.011", "1000", "5", true},
		{"base shifted above tolerance", "0.506", "1000", "500", true},
		{"material conflict", "0.9", "100", "50", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"usages":{"limit_7d":{"used_ratio":` + tc.modern + `}},"usage":{"limit":` + tc.legacyLimit + `,"used":` + tc.legacyUsed + `,"remaining":0}}`
			reading := decodeSynthetic(t, raw)
			quota := reading.Quotas[domain.QuotaLimit7d]
			requireCount(t, quota.Limit, mustFloat(t, tc.legacyLimit))
			requireCount(t, quota.Used, mustFloat(t, tc.legacyUsed))
			if tc.conflict {
				if quota.UsedRatio != nil || quota.ConflictReason != domain.ConflictSources {
					t.Fatalf("conflicting quota = %+v, want nil ratio and conflicting_sources", quota)
				}
				if len(reading.Warnings) != 1 || reading.Warnings[0].Section != "Quota consistency" {
					t.Fatalf("warnings = %v, want Quota consistency", reading.Warnings)
				}
			} else {
				requireRatio(t, quota, mustFloat(t, tc.modern))
				if quota.Source != domain.SourceRatio || quota.ConflictReason != "" {
					t.Fatalf("agreeing quota = %+v, want modern source without conflict", quota)
				}
				requireNoWarnings(t, reading)
			}
		})
	}
}

func TestDecodeOutOfRangeLegacyRatioStillConflictsWithModern(t *testing.T) {
	reading := decodeSynthetic(t, `{"usages":{"limit_7d":{"used_ratio":0.5}},"usage":{"limit":1,"used":2,"remaining":0}}`)
	quota := reading.Quotas[domain.QuotaLimit7d]
	if quota.UsedRatio != nil {
		t.Fatalf("normalized ratio = %v, want nil rather than modern or out-of-range legacy value", *quota.UsedRatio)
	}
	if quota.ConflictReason != domain.ConflictSources {
		t.Fatalf("conflict reason = %q, want conflicting_sources", quota.ConflictReason)
	}
	requireCount(t, quota.Used, 2)
}

func TestDecodeInvalidModernNeverSilentlyFallsBackToLegacy(t *testing.T) {
	for _, modern := range []string{`{"used_ratio":2}`, `[]`} {
		reading := decodeSynthetic(t, `{"usages":{"limit_7d":`+modern+`},"usage":{"limit":100,"used":50,"remaining":50}}`)
		quota := reading.Quotas[domain.QuotaLimit7d]
		if quota.UsedRatio != nil || quota.ConflictReason != domain.ConflictSources {
			t.Fatalf("invalid modern %s plus legacy = %+v, want unavailable conflicting ratio", modern, quota)
		}
		requireCount(t, quota.Used, 50)
	}

	missing := decodeSynthetic(t, `{"usages":{"limit_7d":{"used_ratio":null}},"usage":{"limit":100,"used":50,"remaining":50}}`)
	requireRatio(t, missing.Quotas[domain.QuotaLimit7d], 0.5)
	if missing.Quotas[domain.QuotaLimit7d].ConflictReason != "" {
		t.Fatalf("missing modern ratio must not conflict: %+v", missing.Quotas[domain.QuotaLimit7d])
	}
}

func TestDecodeWarningsAreDeterministicAndRedacted(t *testing.T) {
	raw := `{"usages":{"limit_5h":{"used_ratio":"` + syntheticToken + `"},"limit_7d":42},"unknown":{"token":"` + syntheticToken + `"}}`
	first := decodeSynthetic(t, raw)
	second := decodeSynthetic(t, raw)
	if len(first.Warnings) != 2 || len(second.Warnings) != 2 {
		t.Fatalf("warnings = %v / %v, want two", first.Warnings, second.Warnings)
	}
	for i := range first.Warnings {
		if first.Warnings[i] != second.Warnings[i] {
			t.Fatalf("warning order changed: %v vs %v", first.Warnings, second.Warnings)
		}
		if strings.Contains(first.Warnings[i].Message, syntheticToken) {
			t.Fatalf("warning leaked raw value: %v", first.Warnings[i])
		}
	}
	if string(first.Raw) != raw {
		t.Fatal("unknown raw fields must round-trip even when recognized fields warn")
	}
}

func mustFloat(t *testing.T, value string) float64 {
	t.Helper()
	var number json.Number = json.Number(value)
	got, err := number.Float64()
	if err != nil || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("invalid synthetic number %q", value)
	}
	return got
}
