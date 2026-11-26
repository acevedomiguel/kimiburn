package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/acevedomiguel/burnblocks/kimi"
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

	// History holds the retained local samples per quota, drawn as trends.
	History map[kimi.QuotaKey][]kimi.Sample
	// Meters holds the remaining fraction each meter currently displays while
	// it animates towards the reading. A missing key draws the reading itself.
	Meters map[kimi.QuotaKey]float64
	// Refreshing marks a request in flight; Spinner is its current frame.
	Refreshing bool
	Spinner    string
	// NextRefresh is when the next automatic read is due, if one is armed.
	NextRefresh time.Time
	// HelpOpen swaps the footer for the Help overlay's own bindings.
	HelpOpen bool
}

// BuildDashboardBody renders normalized quota panels in priority order.
// Raw provider JSON is available through --json, not the interactive dashboard.
func BuildDashboardBody(snapshot kimi.Snapshot, opts DashboardOptions) []string {
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

	panelFor := func(title string, key kimi.QuotaKey, caption string, w int, allocation bool) []string {
		quota, ok := snapshot.Quotas[key]
		return quotaPanel(panelSpec{
			title: title, key: key, quota: quota, ok: ok, runway: snapshot.Runways[key],
			caption: caption, width: w, allocation: allocation,
		}, opts)
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		block(panelFor("5-hour quota", kimi.QuotaLimit5h, "", leftWidth, false)),
		" ",
		block(panelFor("Monthly membership", kimi.QuotaLimitMonthTotal, FreshnessCaption(snapshot, opts.Cached, opts.Now), rightWidth, false)),
	)
	lines := strings.Split(top, "\n")

	_, codeOK := snapshot.Quotas[kimi.QuotaLimitMonthCode]
	_, legacyOK := snapshot.Quotas[kimi.QuotaLimit7d]
	if legacyOK && codeOK && width >= 120 {
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			block(panelFor("Monthly Code allocation", kimi.QuotaLimitMonthCode, "", leftWidth, true)),
			" ",
			block(panelFor("7-day quota · legacy", kimi.QuotaLimit7d, "", rightWidth, false)),
		)
		lines = append(lines, strings.Split(row, "\n")...)
	} else {
		// Code allocation keeps the same half width as the panel above it, so the
		// left column lines up at every terminal size.
		if codeOK {
			lines = append(lines, panelFor("Monthly Code allocation", kimi.QuotaLimitMonthCode, "", leftWidth, true)...)
		}
		if legacyOK {
			lines = append(lines, panelFor("7-day quota · legacy", kimi.QuotaLimit7d, "", width, false)...)
		}
	}

	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

// block joins panel rows for the lipgloss layout helpers.
func block(rows []string) string { return strings.Join(rows, "\n") }

// refreshStatus is the right side of the footer: the spinner while a request
// is in flight, otherwise a countdown to the next automatic read.
func refreshStatus(opts DashboardOptions) string {
	switch {
	case opts.Refreshing:
		spin := opts.Spinner
		if spin == "" {
			spin = "…"
		}
		return spin + " " + opts.Styles.Muted.Render("refreshing")
	case !opts.NextRefresh.IsZero() && opts.NextRefresh.After(opts.Now):
		return opts.Styles.Muted.Render("next read " + formatCountdown(opts.NextRefresh.Sub(opts.Now)))
	}
	return ""
}

// formatCountdown renders a short, second-resolution wait.
func formatCountdown(d time.Duration) string {
	seconds := int(math.Ceil(d.Seconds()))
	if seconds < 60 {
		return itoa(seconds) + "s"
	}
	return itoa(seconds/60) + "m " + itoa(seconds%60) + "s"
}

// RenderDashboard returns an exact-height frame with the footer on the last
// row. Body scrolling never changes the reading's timestamp.
func RenderDashboard(snapshot kimi.Snapshot, opts DashboardOptions) string {
	layout := Classify(opts.Width, opts.Height)
	if layout.Class == ClassTooSmall {
		return renderTooSmall(opts)
	}
	if opts.Now.IsZero() {
		opts.Now = snapshot.GeneratedAt
	}

	warning := DashboardWarning(snapshot, opts.Warning)
	bodyHeight := BodyHeight(opts.Height, warning != "")
	body := BuildDashboardBody(snapshot, opts)

	lines := make([]string, 0, opts.Height)
	title := "KIMIBURN · Kimi Code quota and runway"
	if opts.Demo {
		title = "KIMIBURN · SYNTHETIC DATA · no provider calls"
	}
	lines = append(lines, lipgloss.PlaceHorizontal(opts.Width, lipgloss.Center, opts.Styles.Title.Render(truncate(title, opts.Width))))
	if warning != "" {
		lines = append(lines, padRight(truncate(opts.Styles.Warning.Render("Warning · "+warning+" · press ? for details"), opts.Width), opts.Width))
	}

	vp := viewport.New(viewport.WithWidth(layout.ContentWidth), viewport.WithHeight(bodyHeight))
	vp.SetContentLines(body)
	vp.SetYOffset(opts.Scroll)
	for _, line := range strings.Split(vp.View(), "\n") {
		line = padRight(line, layout.ContentWidth)
		if layout.Padding > 0 {
			line = strings.Repeat(" ", layout.Padding) + line + strings.Repeat(" ", layout.Padding)
		}
		lines = append(lines, padRight(line, opts.Width))
	}
	for len(lines) < opts.Height-1 {
		lines = append(lines, strings.Repeat(" ", opts.Width))
	}
	lines = append(lines[:opts.Height-1], Footer(opts))
	return strings.Join(lines[:opts.Height], "\n")
}

// DashboardWarning picks the single warning shown under the title.
func DashboardWarning(snapshot kimi.Snapshot, warning string) string {
	if warning == "" && len(snapshot.Warnings) > 0 {
		warning = snapshot.Warnings[0].Section + ": " + snapshot.Warnings[0].Message
	}
	if _, month := snapshot.Quotas[kimi.QuotaLimitMonthTotal]; !month {
		if _, code := snapshot.Quotas[kimi.QuotaLimitMonthCode]; !code && warning == "" {
			warning = "Parser: monthly quota data was not reported by this account/API response"
		}
	}
	return warning
}

// BodyHeight is the number of rows left for the scrollable panels once the
// title, optional warning, and footer are placed.
func BodyHeight(height int, warning bool) int {
	chrome := 2
	if warning {
		chrome++
	}
	return maxInt(1, height-chrome)
}

// Footer renders the pinned bottom bar: the help bubble's key hints on the
// left and the refresh status on the right.
func Footer(opts DashboardOptions) string {
	width := maxInt(opts.Width, 1)
	right := refreshStatus(opts)
	helpWidth := width - 2
	if right != "" {
		helpWidth -= lipgloss.Width(right) + 1
	}
	h := NewHelp(opts.Styles, maxInt(helpWidth, 1))
	var left string
	if opts.HelpOpen {
		left = h.View(NewHelpKeyMap())
	} else {
		left = h.View(NewKeyMap())
	}
	bar := left
	if right != "" {
		gap := maxInt(width-2-lipgloss.Width(left)-lipgloss.Width(right), 1)
		bar = left + strings.Repeat(" ", gap) + right
	}
	bar = padRight(bar, width-2)
	out := opts.Styles.StatusBar.Render(bar)
	return padRight(out, width)
}

type panelSpec struct {
	title      string
	key        kimi.QuotaKey
	quota      kimi.QuotaReading
	ok         bool
	runway     kimi.RunwayEstimate
	caption    string
	width      int
	allocation bool
}

func quotaPanel(p panelSpec, opts DashboardOptions) []string {
	st := opts.Styles
	textWidth := p.width - 4
	if !p.ok {
		return panelWithCaption(p.title, p.caption, []string{"Not reported by this account/API response"}, p.width, 3, st)
	}

	rows := make([]string, 0, 8)
	if validRatio(p.quota.UsedRatio) {
		remaining := 1 - *p.quota.UsedRatio
		shown, animating := opts.Meters[p.key]
		if !animating {
			shown = remaining
		}
		rows = append(rows, remainingMeter(remaining, shown, textWidth, st))
	} else {
		rows = append(rows, st.Muted.Render("Meter unavailable · ratio not reported or invalid"))
	}
	if row, ok := timelineRow(p.quota, p.runway, textWidth, opts.Now, st); ok {
		rows = append(rows, row)
	}
	runwayStyle := st.Value
	if p.runway.Status == "finite" || p.runway.Status == "depleted" {
		runwayStyle = st.Error.Bold(st.Enabled)
	}
	rows = append(rows,
		leader("Reset", FormatReset(p.quota.ResetAt, opts.Now), textWidth, st),
		leaderValue("Runway", FormatRunway(p.runway, p.key), textWidth, st, runwayStyle),
		leader("Burn", FormatBurn(p.runway, p.key), textWidth, st),
	)
	if row, ok := trendRow(currentPeriod(opts.History[p.key], p.quota.ResetAt), textWidth, opts.Now, st); ok {
		rows = append(rows, row)
	}
	if p.quota.Used != nil && p.quota.Limit != nil {
		counts := FormatCount(p.quota.Used) + "/" + FormatCount(p.quota.Limit) + " used"
		if p.quota.Remaining != nil {
			counts += " · " + FormatCount(p.quota.Remaining) + " left"
		}
		rows = append(rows, leader("Requests", counts, textWidth, st))
	}
	if p.allocation {
		rows = append(rows, st.Muted.Render("Included in membership"))
	}
	return panelWithCaption(p.title, p.caption, rows, p.width, len(rows)+2, st)
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

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return itoa(v/10) + string(rune('0'+v%10))
}

// currentPeriod keeps only the samples that belong to the quota's current reset
// period, so a cached history from before a reset never draws as this period's
// trend.
func currentPeriod(samples []kimi.Sample, reset *time.Time) []kimi.Sample {
	if reset == nil {
		return nil
	}
	out := make([]kimi.Sample, 0, len(samples))
	for _, sample := range samples {
		if sample.ResetAt.Equal(*reset) {
			out = append(out, sample)
		}
	}
	return out
}
