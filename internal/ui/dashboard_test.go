package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func TestDashboardBodyShowsCoreFactsWithoutRawAdditionalData(t *testing.T) {
	snapshot := SyntheticLegacySnapshot()
	body := strings.Join(BuildDashboardBody(snapshot, DashboardOptions{Width: 100, Height: 40, Now: snapshot.GeneratedAt.Add(time.Minute), Styles: NewStyles(false)}), "\n")
	for _, want := range []string{
		"Monthly membership", "5-hour quota", "Kimi Code monthly", "7-day quota · legacy",
		"41.8%", "72.0%", "Allocation within shared membership · not additive",
		"121 · 1h 0m · high", "60 · 29m · medium",
		"30 · 15m · medium", "Used 40/100 requests",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	if !regexp.MustCompile(`Remaining ·+ 60`).MatchString(body) {
		t.Errorf("legacy remaining count missing\n%s", body)
	}
	for _, unwanted := range []string{"Additional API data", "future_provider_field", "synthetic-passthrough-value", `"limit_5h"`, `"limit_month_total"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("dashboard exposes raw provider JSON %q:\n%s", unwanted, body)
		}
	}
}

func TestLegacyPanelCollapsesAndMissingMonthlyCodeRemainsExplained(t *testing.T) {
	snapshot := SyntheticSnapshot()
	body := strings.Join(BuildDashboardBody(snapshot, DashboardOptions{Width: 80, Height: 24, Now: snapshot.GeneratedAt, Styles: NewStyles(false)}), "\n")
	if strings.Contains(body, "7-day quota · legacy") {
		t.Error("absent legacy panel did not collapse")
	}
	for _, want := range []string{"Kimi Code monthly", "Not reported by this account/API response", "Allocation within shared membership · not additive"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
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
