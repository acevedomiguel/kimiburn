package ui

import (
	"encoding/json"
	"math"
	"strings"
	"time"

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

// BuildDashboardBody renders every panel in priority order. Scrolling is a
// frame concern, so additional API data is never silently discarded.
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
		quotaPanel("Monthly membership", domain.QuotaLimitMonthTotal, month, monthOK, snapshot.Runways[domain.QuotaLimitMonthTotal], FreshnessCaption(snapshot, opts.Cached, opts.Now), leftWidth, opts.Now, opts.Styles, false),
		quotaPanel("5-hour quota", domain.QuotaLimit5h, five, fiveOK, snapshot.Runways[domain.QuotaLimit5h], "", rightWidth, opts.Now, opts.Styles, false),
	)
	lines := append([]string{}, top...)

	code, codeOK := snapshot.Quotas[domain.QuotaLimitMonthCode]
	legacy, legacyOK := snapshot.Quotas[domain.QuotaLimit7d]
	if legacyOK && width >= 120 {
		lines = append(lines, joinColumns(
			quotaPanel("Kimi Code monthly", domain.QuotaLimitMonthCode, code, codeOK, snapshot.Runways[domain.QuotaLimitMonthCode], "", leftWidth, opts.Now, opts.Styles, true),
			quotaPanel("7-day quota · legacy", domain.QuotaLimit7d, legacy, true, snapshot.Runways[domain.QuotaLimit7d], "", rightWidth, opts.Now, opts.Styles, false),
		)...)
	} else {
		lines = append(lines, quotaPanel("Kimi Code monthly", domain.QuotaLimitMonthCode, code, codeOK, snapshot.Runways[domain.QuotaLimitMonthCode], "", width, opts.Now, opts.Styles, true)...)
		if legacyOK {
			lines = append(lines, quotaPanel("7-day quota · legacy", domain.QuotaLimit7d, legacy, true, snapshot.Runways[domain.QuotaLimit7d], "", width, opts.Now, opts.Styles, false)...)
		}
	}

	additional := additionalDataLines(snapshot.Raw, width-4)
	lines = append(lines, panel("Additional API data", additional, width, len(additional)+2, opts.Styles)...)
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
	title := "KIMIBURN · Kimi Code quota and local runway"
	if opts.Demo {
		title = "KIMIBURN · SYNTHETIC DATA · no provider calls"
	}
	lines = append(lines, padRight(truncate(opts.Styles.Title.Render(title), opts.Width), opts.Width))
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
		rows := []string{"Not reported by this account/API response", leader("Absolute amount", "Not reported", textWidth, st)}
		if allocation {
			rows = append(rows, "Allocation within shared membership · not additive")
		}
		return panelWithCaption(title, caption, rows, width, len(rows)+2, st)
	}

	rows := []string{
		leader("Used", FormatPercent(quota.UsedRatio), textWidth, st),
		leader("Remaining", FormatPercent(remainingPointer(quota.UsedRatio)), textWidth, st),
	}
	if validRatio(quota.UsedRatio) {
		rows = append(rows, meter(*quota.UsedRatio, textWidth, st))
	} else {
		rows = append(rows, st.Muted.Render("Meter unavailable · ratio not reported or invalid"))
	}
	resetValue := FormatReset(quota.ResetAt, now)
	if textWidth >= 48 && quota.ResetAt != nil && quota.ResetAt.After(now) {
		resetValue += " · " + quota.ResetAt.UTC().Format("2006-01-02 15:04Z")
	}
	rows = append(rows,
		leader("Reset", resetValue, textWidth, st),
		leader("Local runway", FormatRunway(runway, key), textWidth, st),
	)
	if runway.Status == "finite" {
		rows = append(rows, leader("Exhausts", FormatExhaustion(runway), textWidth, st))
	}
	rows = append(rows, leader("Burn", FormatBurn(runway, key), textWidth, st))
	if quota.Used != nil && quota.Limit != nil {
		rows = append(rows, leader("Absolute", "Used "+FormatCount(quota.Used)+"/"+FormatCount(quota.Limit)+" requests", textWidth, st))
		if quota.Remaining != nil {
			rows = append(rows, leader("Remaining", FormatCount(quota.Remaining), textWidth, st))
		}
	} else {
		rows = append(rows, leader("Absolute amount", "Not reported", textWidth, st))
	}
	if allocation {
		rows = append(rows, "Allocation within shared membership · not additive")
	}
	return panelWithCaption(title, caption, rows, width, len(rows)+2, st)
}

func remainingPointer(used *float64) *float64 {
	if !validRatio(used) {
		return nil
	}
	remaining := 1 - *used
	return &remaining
}

func validRatio(ratio *float64) bool {
	return ratio != nil && !math.IsNaN(*ratio) && !math.IsInf(*ratio, 0) && *ratio >= 0 && *ratio <= 1
}

func additionalDataLines(raw json.RawMessage, width int) []string {
	top := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &top)
	if usagesRaw, ok := top["usages"]; ok {
		usages := map[string]json.RawMessage{}
		if json.Unmarshal(usagesRaw, &usages) == nil {
			for _, key := range domain.CanonicalQuotaKeys {
				delete(usages, string(key))
			}
			if len(usages) == 0 {
				delete(top, "usages")
			} else {
				encoded, _ := json.Marshal(usages)
				top["usages"] = encoded
			}
		}
	}
	if len(top) == 0 {
		return []string{"No additional fields reported"}
	}
	pretty, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return []string{"Additional data could not be rendered"}
	}
	return wrapPlain(string(pretty), width)
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
