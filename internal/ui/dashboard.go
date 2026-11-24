package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// DashboardOptions controls one pure dashboard frame.
type DashboardOptions struct {
	Width, Height int
	Now           time.Time
	Cached        bool
	Demo          bool
	Styles        Styles
	Warning       string
	Scroll        int
}

// BuildDashboardBody renders normalized quota panels in priority order.
// Raw provider JSON is available through --json, not the interactive dashboard.
func BuildDashboardBody(snapshot domain.Snapshot, opts DashboardOptions) []string {
	layout := Classify(opts.Width, opts.Height)
	if layout.Class == ClassTooSmall {
		return []string{TooSmallMessage}
	}
	if opts.Now.IsZero() {
		opts.Now = snapshot.GeneratedAt
	}
	width := layout.ContentWidth
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth

	month, monthOK := snapshot.Quotas[domain.QuotaLimitMonthTotal]
	five, fiveOK := snapshot.Quotas[domain.QuotaLimit5h]
	top := joinColumns(
		quotaPanel("5-hour quota", domain.QuotaLimit5h, five, fiveOK, snapshot.Runways[domain.QuotaLimit5h], "", leftWidth, opts.Now, opts.Styles, false),
		quotaPanel("Monthly membership", domain.QuotaLimitMonthTotal, month, monthOK, snapshot.Runways[domain.QuotaLimitMonthTotal], FreshnessCaption(snapshot, opts.Cached, opts.Now), rightWidth, opts.Now, opts.Styles, false),
	)
	lines := append([]string{}, top...)

	code, codeOK := snapshot.Quotas[domain.QuotaLimitMonthCode]
	legacy, legacyOK := snapshot.Quotas[domain.QuotaLimit7d]
	if legacyOK && codeOK && width >= 120 {
		lines = append(lines, joinColumns(
			quotaPanel("Monthly Code allocation", domain.QuotaLimitMonthCode, code, true, snapshot.Runways[domain.QuotaLimitMonthCode], "", leftWidth, opts.Now, opts.Styles, true),
			quotaPanel("7-day quota · legacy", domain.QuotaLimit7d, legacy, true, snapshot.Runways[domain.QuotaLimit7d], "", rightWidth, opts.Now, opts.Styles, false),
		)...)
	} else {
		// Code allocation keeps the same half width as the panel above it, so the
		// left column lines up at every terminal size.
		if codeOK {
			lines = append(lines, quotaPanel("Monthly Code allocation", domain.QuotaLimitMonthCode, code, true, snapshot.Runways[domain.QuotaLimitMonthCode], "", leftWidth, opts.Now, opts.Styles, true)...)
		}
		if legacyOK {
			lines = append(lines, quotaPanel("7-day quota · legacy", domain.QuotaLimit7d, legacy, true, snapshot.Runways[domain.QuotaLimit7d], "", width, opts.Now, opts.Styles, false)...)
		}
	}

	return lines
}

// RenderDashboard returns an exact-height frame with the footer on the last
// row. Body scrolling never changes the reading's timestamp.
func RenderDashboard(snapshot domain.Snapshot, opts DashboardOptions) string {
	layout := Classify(opts.Width, opts.Height)
	if layout.Class == ClassTooSmall {
		return renderTooSmall(opts)
	}
	if opts.Now.IsZero() {
		opts.Now = snapshot.GeneratedAt
	}

	warning := opts.Warning
	if warning == "" && len(snapshot.Warnings) > 0 {
		warning = snapshot.Warnings[0].Section + ": " + snapshot.Warnings[0].Message
	}
	if _, month := snapshot.Quotas[domain.QuotaLimitMonthTotal]; !month {
		if _, code := snapshot.Quotas[domain.QuotaLimitMonthCode]; !code && warning == "" {
			warning = "Parser: monthly quota data was not reported by this account/API response"
		}
	}

	chrome := 2
	if warning != "" {
		chrome++
	}
	bodyHeight := maxInt(1, opts.Height-chrome)
	body := BuildDashboardBody(snapshot, opts)
	scroll := minInt(maxInt(0, opts.Scroll), maxInt(0, len(body)-bodyHeight))
	end := minInt(len(body), scroll+bodyHeight)

	lines := make([]string, 0, opts.Height)
	title := "KIMIBURN · Kimi Code quota and runway"
	if opts.Demo {
		title = "KIMIBURN · SYNTHETIC DATA · no provider calls"
	}
	styledTitle := opts.Styles.Title.Render(title)
	if left := (opts.Width - lipgloss.Width(title)) / 2; left > 0 {
		styledTitle = strings.Repeat(" ", left) + styledTitle
	}
	lines = append(lines, padRight(truncate(styledTitle, opts.Width), opts.Width))
	if warning != "" {
		lines = append(lines, padRight(truncate(opts.Styles.Warning.Render("Warning · "+warning+" · press ? for details"), opts.Width), opts.Width))
	}
	for _, line := range body[scroll:end] {
		line = padRight(line, layout.ContentWidth)
		if layout.Padding > 0 {
			line = strings.Repeat(" ", layout.Padding) + line + strings.Repeat(" ", layout.Padding)
		}
		lines = append(lines, padRight(line, opts.Width))
	}
	for len(lines) < opts.Height-1 {
		lines = append(lines, strings.Repeat(" ", opts.Width))
	}
	footer := padRight(truncate("[r] refresh · [↑↓/j/k] scroll · [pgup/pgdn] page · [?] help · [q] quit", opts.Width-2), opts.Width-2)
	lines = append(lines, opts.Styles.StatusBar.Render(footer))
	return strings.Join(lines[:opts.Height], "\n")
}

func quotaPanel(title string, key domain.QuotaKey, quota domain.QuotaReading, ok bool, runway domain.RunwayEstimate, caption string, width int, now time.Time, st Styles, allocation bool) []string {
	textWidth := width - 4
	if !ok {
		return panelWithCaption(title, caption, []string{"Not reported by this account/API response"}, width, 3, st)
	}

	rows := make([]string, 0, 6)
	if validRatio(quota.UsedRatio) {
		rows = append(rows, remainingMeter(*quota.UsedRatio, textWidth, st))
	} else {
		rows = append(rows, st.Muted.Render("Meter unavailable · ratio not reported or invalid"))
	}
	resetValue := FormatReset(quota.ResetAt, now)
	rows = append(rows,
		leader("Reset", resetValue, textWidth, st),
		leader("Runway", FormatRunway(runway, key), textWidth, st),
	)
	rows = append(rows, leader("Burn", FormatBurn(runway, key), textWidth, st))
	if quota.Used != nil && quota.Limit != nil {
		counts := FormatCount(quota.Used) + "/" + FormatCount(quota.Limit) + " used"
		if quota.Remaining != nil {
			counts += " · " + FormatCount(quota.Remaining) + " left"
		}
		rows = append(rows, leader("Requests", counts, textWidth, st))
	}
	if allocation {
		rows = append(rows, "Included in membership")
	}
	return panelWithCaption(title, caption, rows, width, len(rows)+2, st)
}

func validRatio(ratio *float64) bool {
	return ratio != nil && !math.IsNaN(*ratio) && !math.IsInf(*ratio, 0) && *ratio >= 0 && *ratio <= 1
}

func renderTooSmall(opts DashboardOptions) string {
	width, height := maxInt(opts.Width, 1), maxInt(opts.Height, 1)
	lines := []string{truncate(TooSmallMessage, width)}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = padRight(lines[i], width)
	}
	return strings.Join(lines, "\n")
}
