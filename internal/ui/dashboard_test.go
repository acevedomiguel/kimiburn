package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/acevedomiguel/burnblocks/kimi"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func TestDashboardBodyShowsCoreFactsWithoutRawAdditionalData(t *testing.T) {
	snapshot := SyntheticLegacySnapshot()
	snapshot.Quotas[kimi.QuotaLimitMonthCode] = kimi.QuotaReading{UsedRatio: floatPointer(0.2)}
	body := strings.Join(BuildDashboardBody(snapshot, DashboardOptions{Width: 100, Height: 40, Now: snapshot.GeneratedAt.Add(time.Minute), Styles: NewStyles(false)}), "\n")
	for _, want := range []string{
		"Monthly membership", "5-hour quota", "Monthly Code allocation", "7-day quota · legacy",
		"58.2%", "28.0%", "Included in membership",
		"Beyond this reset", "2h 20m", "Requests", "40/100 used · 60 left",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"Additional API data", "future_provider_field", "synthetic-passthrough-value", `"limit_5h"`, `"limit_month_total"`, "Samples", "confidence", "Absolute amount", "Local runway", "Reset in", "┬"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("dashboard exposes raw provider JSON %q:\n%s", unwanted, body)
		}
	}
}

func TestLegacyAndMissingMonthlyCodePanelsCollapse(t *testing.T) {
	snapshot := SyntheticSnapshot()
	body := strings.Join(BuildDashboardBody(snapshot, DashboardOptions{Width: 80, Height: 24, Now: snapshot.GeneratedAt, Styles: NewStyles(false)}), "\n")
	if strings.Contains(body, "7-day quota · legacy") {
		t.Error("absent legacy panel did not collapse")
	}
	if strings.Contains(body, "Monthly Code allocation") || strings.Contains(body, "Not reported by this account/API response") {
		t.Errorf("missing Code allocation should not render an empty panel:\n%s", body)
	}
}

func TestDashboardExactLayoutsAndNoColor(t *testing.T) {
	snapshot := SyntheticLegacySnapshot()
	for _, size := range []struct{ w, h int }{{60, 16}, {80, 24}, {100, 40}, {140, 20}} {
		frame := RenderDashboard(snapshot, DashboardOptions{Width: size.w, Height: size.h, Now: snapshot.GeneratedAt, Styles: NewStyles(false)})
		lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d: %d rows, want %d", size.w, size.h, len(lines), size.h)
		}
		for i, line := range lines {
			if ansiPattern.MatchString(line) {
				t.Errorf("%dx%d row %d contains ANSI", size.w, size.h, i)
			}
			if lipgloss.Width(line) > size.w {
				t.Errorf("%dx%d row %d width=%d: %q", size.w, size.h, i, lipgloss.Width(line), line)
			}
		}
		if !strings.Contains(lines[len(lines)-1], "[r] refresh") {
			t.Errorf("%dx%d footer not pinned: %q", size.w, size.h, lines[len(lines)-1])
		}
	}
}

func TestDashboardTooSmall(t *testing.T) {
	frame := RenderDashboard(SyntheticSnapshot(), DashboardOptions{Width: 59, Height: 15, Styles: NewStyles(false)})
	if !strings.Contains(frame, TooSmallMessage) {
		t.Fatalf("frame=%q", frame)
	}
}

func TestTopRowPutsFiveHourLeftOfMonthlyMembership(t *testing.T) {
	snapshot := SyntheticLegacySnapshot()
	body := BuildDashboardBody(snapshot, DashboardOptions{Width: 100, Height: 40, Now: snapshot.GeneratedAt, Styles: NewStyles(false)})
	if len(body) == 0 {
		t.Fatal("empty dashboard body")
	}
	top := body[0]
	five, month := strings.Index(top, "5-hour quota"), strings.Index(top, "Monthly membership")
	if five < 0 || month < 0 {
		t.Fatalf("top row missing a panel: %q", top)
	}
	if five > month {
		t.Errorf("5-hour quota must sit left of Monthly membership:\n%s", top)
	}
}

func TestMonthlyCodeAllocationKeepsHalfWidth(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		legacy bool
	}{
		{"no legacy 80", 80, false},
		{"no legacy 100", 100, false},
		{"no legacy 140", 140, false},
		{"with legacy 100", 100, true},
	} {
		snapshot := SyntheticSnapshot()
		if tc.legacy {
			snapshot = SyntheticLegacySnapshot()
		}
		snapshot.Quotas[kimi.QuotaLimitMonthCode] = kimi.QuotaReading{UsedRatio: floatPointer(0.2)}
		body := BuildDashboardBody(snapshot, DashboardOptions{Width: tc.width, Height: 40, Now: snapshot.GeneratedAt, Styles: NewStyles(false)})

		// leftWidth is the width of the 5-hour quota panel, which now occupies
		// the left column of the top row.
		want := (Classify(tc.width, 40).ContentWidth - 1) / 2

		var codeLine string
		for _, line := range body {
			if strings.Contains(line, "Monthly Code allocation") {
				codeLine = line
				break
			}
		}
		if codeLine == "" {
			t.Fatalf("%s: Monthly Code allocation panel not rendered", tc.name)
		}
		if got := lipgloss.Width(codeLine); got != want {
			t.Errorf("%s: Code allocation width = %d, want %d (same as the 5-hour quota panel)\n%s", tc.name, got, want, codeLine)
		}
	}
}
