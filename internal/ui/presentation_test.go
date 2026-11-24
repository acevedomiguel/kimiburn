package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestDashboardCentersTitleAndOmitsAbsoluteQuotaTimes(t *testing.T) {
	frame := RenderDashboard(SyntheticSnapshot(), DashboardOptions{Width: 120, Height: 40, Demo: true, Styles: NewStyles(false)})
	first := strings.Split(frame, "\n")[0]
	const title = "KIMIBURN · SYNTHETIC DATA · no provider calls"
	left := strings.Index(first, title)
	if left < 0 {
		t.Fatalf("title is missing: %q", first)
	}
	right := lipgloss.Width(first) - left - lipgloss.Width(title)
	if difference := left - right; difference < -1 || difference > 1 {
		t.Errorf("title is not centered: left=%d right=%d", left, right)
	}
	for _, unwanted := range []string{"2026-12-12", "2026-11-24", "Exhausts", "16:52:30"} {
		if strings.Contains(frame, unwanted) {
			t.Errorf("dashboard still contains absolute quota time %q", unwanted)
		}
	}
}

func TestRemainingMeterPlacesLabelBarAndPercentageInColumns(t *testing.T) {
	const width = 40
	got := remainingMeter(0.582, 0.582, width, NewStyles(false))
	if lipgloss.Width(got) != width {
		t.Fatalf("meter width=%d, want %d: %q", lipgloss.Width(got), width, got)
	}
	if !strings.HasPrefix(got, "Remaining ▇") {
		t.Errorf("remaining label and bar are not on the left: %q", got)
	}
	if !strings.HasSuffix(got, " 58.2%") {
		t.Errorf("remaining percentage is not on the right: %q", got)
	}
	if strings.Contains(got, "41.8%") {
		t.Errorf("meter still displays used percentage: %q", got)
	}
	if !strings.Contains(got, "░") {
		t.Errorf("meter lost its unfilled portion: %q", got)
	}
}

func TestPanelTitleUsesPlainHorizontalRules(t *testing.T) {
	got := strings.Join(panel("Monthly membership", []string{"row"}, 40, 3, NewStyles(false)), "\n")
	if strings.Contains(got, "┬") {
		t.Fatalf("panel contains the old title separator: %q", got)
	}
	if !strings.Contains(got, "─ Monthly membership ─") {
		t.Fatalf("panel title does not use plain horizontal rules: %q", got)
	}
}
