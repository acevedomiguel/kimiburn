package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// sparkGlyphs are the eight block heights used by the usage trend.
var sparkGlyphs = []rune("▁▂▃▄▅▆▇█")

// minTrendSpan is the shortest sample window worth drawing as a trend.
const minTrendSpan = time.Minute

// timelineRow answers the panel's main question on one axis: does the quota
// run out before it resets? "now" is the left edge and the reset is the right
// edge; ✕ marks the estimated depletion. Only relative positions are drawn, so
// no absolute quota time appears on screen. It returns false when there is no
// estimate or no future reset to compare against.
func timelineRow(quota domain.QuotaReading, runway domain.RunwayEstimate, width int, now time.Time, st Styles) (string, bool) {
	if quota.ResetAt == nil || !quota.ResetAt.After(now) {
		return "", false
	}
	const prefix, suffix = "now ", " reset"
	safe := runway.Status == "beyond_reset" || runway.Status == "no_consumption"
	tail := suffix
	if safe {
		tail += " ✓"
	}
	track := width - lipgloss.Width(prefix) - lipgloss.Width(tail)
	if track < 6 {
		return "", false
	}
	// The track is ● + path + │; the path holds the inner cells.
	inner := track - 2

	switch {
	case safe:
		path := st.Success.Render(strings.Repeat("━", inner))
		return st.Label.Render(prefix) + st.Success.Render("●") + path + st.Muted.Render("│") + st.Label.Render(suffix) + st.Success.Render(" ✓"), true
	case runway.Status == "depleted":
		path := st.Muted.Render(strings.Repeat("┄", inner))
		return st.Label.Render(prefix) + st.Error.Render("✕") + path + st.Muted.Render("│") + st.Label.Render(suffix), true
	case runway.Status == "finite" && runway.Seconds != nil:
		horizon := quota.ResetAt.Sub(now).Seconds()
		if horizon <= 0 {
			return "", false
		}
		ratio := clamp01(*runway.Seconds / horizon)
		pos := int(ratio*float64(inner) + 0.5)
		if pos >= inner {
			pos = inner - 1
		}
		if pos < 0 {
			pos = 0
		}
		run := st.Warning.Render(strings.Repeat("━", pos))
		rest := st.Muted.Render(strings.Repeat("┄", inner-pos-1))
		return st.Label.Render(prefix) + st.Accent.Render("●") + run + st.Error.Render("✕") + rest + st.Muted.Render("│") + st.Label.Render(suffix), true
	}
	return "", false
}

// trendRow draws the retained local samples of the current reset period as a
// sparkline of used quota, followed by how much was used over that span. The
// samples are the same ones the runway estimate is derived from, so the slope
// on screen is the slope behind the forecast.
func trendRow(samples []domain.Sample, width int, now time.Time, st Styles) (string, bool) {
	if len(samples) < 2 {
		return "", false
	}
	first, last := samples[0], samples[len(samples)-1]
	span := last.ObservedAt.Sub(first.ObservedAt)
	if span < minTrendSpan {
		return "", false
	}
	const label = "Usage"
	delta := (last.UsedRatio - first.UsedRatio) * 100
	sign := "+"
	if delta < 0 {
		sign = "-"
		delta = -delta
	}
	precision := 1
	if delta < 1 {
		precision = 2
	}
	value := fmt.Sprintf("%s%.*f%% in %s", sign, precision, delta, formatDuration(span, true))
	barWidth := width - lipgloss.Width(label) - lipgloss.Width(value) - 2
	if barWidth < 4 {
		return "", false
	}
	return st.Label.Render(label) + " " + st.Accent.Render(sparkline(samples, barWidth)) + " " + st.Value.Render(value), true
}

// sparkline resamples the series into width time buckets, carrying the last
// reading forward, and scales the buckets between the window's lowest and
// highest used ratio so a small but steady burn is still visible.
func sparkline(samples []domain.Sample, width int) string {
	start := samples[0].ObservedAt
	span := samples[len(samples)-1].ObservedAt.Sub(start)
	values := make([]float64, width)
	next := 0
	current := samples[0].UsedRatio
	for i := range values {
		edge := start.Add(time.Duration(float64(span) * float64(i+1) / float64(width)))
		for next < len(samples) && !samples[next].ObservedAt.After(edge) {
			current = samples[next].UsedRatio
			next++
		}
		values[i] = current
	}

	low, high := values[0], values[0]
	for _, v := range values {
		low = min(low, v)
		high = max(high, v)
	}
	var b strings.Builder
	top := len(sparkGlyphs) - 1
	for _, v := range values {
		level := 0
		if high-low > 0.0005 {
			level = int((v-low)/(high-low)*float64(top) + 0.5)
		}
		b.WriteRune(sparkGlyphs[level])
	}
	return b.String()
}
