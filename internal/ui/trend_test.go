package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

func TestTimelinePlacesDepletionBeforeReset(t *testing.T) {
	now := syntheticTime
	reset := now.Add(4 * time.Hour)
	quota := domain.QuotaReading{ResetAt: &reset}
	st := NewStyles(false)

	half := 2 * 3600.0
	row, ok := timelineRow(quota, domain.RunwayEstimate{Status: "finite", Seconds: &half}, 40, now, st)
	if !ok || lipgloss.Width(row) != 40 {
		t.Fatalf("finite row ok=%t width=%d: %q", ok, lipgloss.Width(row), row)
	}
	cross := strings.Index(row, "✕")
	bar := strings.Index(row, "│")
	if cross < 0 || bar < 0 || cross > bar || strings.Contains(row, "✓") {
		t.Fatalf("depletion is not drawn before the reset: %q", row)
	}

	for _, status := range []string{"beyond_reset", "no_consumption"} {
		row, ok = timelineRow(quota, domain.RunwayEstimate{Status: status}, 40, now, st)
		if !ok || !strings.HasSuffix(row, "reset ✓") || strings.Contains(row, "✕") || lipgloss.Width(row) != 40 {
			t.Fatalf("%s row: %q", status, row)
		}
	}

	row, ok = timelineRow(quota, domain.RunwayEstimate{Status: "depleted"}, 40, now, st)
	if !ok || !strings.HasPrefix(row, "now ✕") {
		t.Fatalf("depleted row: %q", row)
	}

	for name, args := range map[string]struct {
		quota  domain.QuotaReading
		runway domain.RunwayEstimate
	}{
		"unavailable":  {quota, domain.RunwayEstimate{Status: "unavailable"}},
		"no reset":     {domain.QuotaReading{}, domain.RunwayEstimate{Status: "beyond_reset"}},
		"passed reset": {domain.QuotaReading{ResetAt: &now}, domain.RunwayEstimate{Status: "beyond_reset"}},
	} {
		if row, ok := timelineRow(args.quota, args.runway, 40, now, st); ok {
			t.Errorf("%s: drew a timeline without a comparison: %q", name, row)
		}
	}
}

func TestTrendNeedsASpanAndReportsUsedChange(t *testing.T) {
	st := NewStyles(false)
	start := syntheticTime
	samples := []domain.Sample{
		{ObservedAt: start, UsedRatio: 0.10},
		{ObservedAt: start.Add(10 * time.Minute), UsedRatio: 0.12},
		{ObservedAt: start.Add(20 * time.Minute), UsedRatio: 0.20},
	}
	row, ok := trendRow(samples, 40, start, st)
	if !ok || lipgloss.Width(row) != 40 {
		t.Fatalf("trend ok=%t width=%d: %q", ok, lipgloss.Width(row), row)
	}
	if !strings.HasPrefix(row, "Usage ▁") || !strings.HasSuffix(row, "+10.0% in 20m") || !strings.Contains(row, "█") {
		t.Fatalf("trend row: %q", row)
	}

	small := []domain.Sample{{ObservedAt: start, UsedRatio: 0.1}, {ObservedAt: start.Add(time.Hour), UsedRatio: 0.1004}}
	if row, _ := trendRow(small, 40, start, st); !strings.HasSuffix(row, "+0.04% in 1h 0m") {
		t.Fatalf("sub-percent change lost its precision: %q", row)
	}

	if _, ok := trendRow(samples[:1], 40, start, st); ok {
		t.Error("one sample drew a trend")
	}
	short := []domain.Sample{{ObservedAt: start}, {ObservedAt: start.Add(30 * time.Second)}}
	if _, ok := trendRow(short, 40, start, st); ok {
		t.Error("a sub-minute span drew a trend")
	}
}

func TestTrendIgnoresSamplesFromAnEarlierResetPeriod(t *testing.T) {
	old, current := syntheticTime, syntheticTime.Add(5*time.Hour)
	samples := []domain.Sample{
		{ObservedAt: syntheticTime, ResetAt: old},
		{ObservedAt: syntheticTime.Add(time.Minute), ResetAt: current},
		{ObservedAt: syntheticTime.Add(2 * time.Minute), ResetAt: current},
	}
	if got := currentPeriod(samples, &current); len(got) != 2 {
		t.Fatalf("kept %d samples, want 2", len(got))
	}
	if got := currentPeriod(samples, nil); len(got) != 0 {
		t.Fatalf("kept %d samples without a reset", len(got))
	}
}

func TestFooterShowsCountdownOrSpinner(t *testing.T) {
	opts := DashboardOptions{Width: 100, Height: 30, Now: syntheticTime, Styles: NewStyles(false), NextRefresh: syntheticTime.Add(90 * time.Second)}
	if footer := Footer(opts); !strings.Contains(footer, "next read 1m 30s") || lipgloss.Width(footer) != 100 {
		t.Fatalf("countdown footer: %q", footer)
	}
	opts.Refreshing, opts.Spinner = true, "⠋"
	if footer := Footer(opts); !strings.Contains(footer, "⠋ refreshing") || strings.Contains(footer, "next read") {
		t.Fatalf("refreshing footer: %q", footer)
	}
}
