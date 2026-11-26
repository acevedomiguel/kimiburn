package ui

import (
	"testing"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

func TestFormattingDistinguishesZeroMissingAndUnavailable(t *testing.T) {
	zero := 0.0
	if got := FormatPercent(&zero); got != "0.0%" {
		t.Errorf("zero percent=%q", got)
	}
	if got := FormatPercent(nil); got != "Not reported" {
		t.Errorf("missing percent=%q", got)
	}
	if got := FormatCount(&zero); got != "0" {
		t.Errorf("zero count=%q", got)
	}
	if got := FormatCount(nil); got != "Not reported" {
		t.Errorf("missing count=%q", got)
	}
	runway := kimi.RunwayEstimate{Status: "depleted", Confidence: "unavailable"}
	if got := FormatRunway(runway, kimi.QuotaLimit5h); got != "Depleted" {
		t.Errorf("depleted=%q", got)
	}
}

func TestLiveCaptionUsesReadingLocalTimezone(t *testing.T) {
	generated := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	location := time.FixedZone("EST", -5*60*60)
	now := generated.In(location)
	got := FreshnessCaption(kimi.Snapshot{GeneratedAt: generated}, false, now)
	if got != "Live · 09:32:30 EST" {
		t.Fatalf("live caption=%q, want local timezone", got)
	}
}

func TestResetAndRunwayFormatting(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 32, 30, 0, time.UTC)
	reset := now.Add(3*time.Hour + 12*time.Minute)
	if got := FormatReset(&reset, now); got != "in 3h 12m" {
		t.Errorf("reset=%q", got)
	}
	overdue := now.Add(-time.Second)
	if got := FormatReset(&overdue, now); got != "Passed · awaiting API update" {
		t.Errorf("overdue=%q", got)
	}
	if got := FormatReset(nil, now); got != "Not reported" {
		t.Errorf("missing reset=%q", got)
	}
	seconds := 8400.0
	finite := kimi.RunwayEstimate{Status: "finite", Seconds: &seconds, Confidence: "medium"}
	if got := FormatRunway(finite, kimi.QuotaLimit5h); got != "2h 20m" {
		t.Errorf("finite=%q", got)
	}
	seconds = 274800.0
	finite.Seconds = &seconds
	if got := FormatRunway(finite, kimi.QuotaLimit5h); got != "3d 4h" {
		t.Errorf("long finite=%q", got)
	}
	burn := 0.00003333333333333333
	finite.BurnRatioPerSecond = &burn
	if got := FormatBurn(finite, kimi.QuotaLimit5h); got != "12.0%/h" {
		t.Errorf("hour burn=%q", got)
	}
	burn = 0.00000010392857142857143
	finite.BurnRatioPerSecond = &burn
	if got := FormatBurn(finite, kimi.QuotaLimitMonthTotal); got != "0.90%/day" {
		t.Errorf("day burn=%q", got)
	}
}
