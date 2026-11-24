package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestMeterPlacesUsedAndRemainingPercentagesInsideBar(t *testing.T) {
	const width = 40
	got := meter(0.418, width, NewStyles(false))
	if lipgloss.Width(got) != width {
		t.Fatalf("meter width=%d, want %d: %q", lipgloss.Width(got), width, got)
	}
	if !strings.HasPrefix(got, "41.8%") {
		t.Errorf("used percentage is not inside the left edge: %q", got)
	}
	if !strings.HasSuffix(got, "58.2%") {
		t.Errorf("remaining percentage is not inside the right edge: %q", got)
	}
	if !strings.Contains(got, "▇") || !strings.Contains(got, "░") {
		t.Errorf("meter lost its proportional fill: %q", got)
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
