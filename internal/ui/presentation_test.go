package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRemainingMeterPlacesLabelBarAndPercentageInColumns(t *testing.T) {
	const width = 40
	got := remainingMeter(0.418, width, NewStyles(false))
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
