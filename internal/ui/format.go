package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// FormatPercent renders a valid ratio at one decimal place. Missing and
// invalid values never become a plausible-looking zero.
func FormatPercent(ratio *float64) string {
	if ratio == nil || math.IsNaN(*ratio) || math.IsInf(*ratio, 0) || *ratio < 0 || *ratio > 1 {
		if ratio == nil {
			return "Not reported"
		}
		return "Unavailable"
	}
	return fmt.Sprintf("%.1f%%", *ratio*100)
}

// FormatCount renders only an actually reported absolute count.
func FormatCount(count *float64) string {
	if count == nil || math.IsNaN(*count) || math.IsInf(*count, 0) || *count < 0 {
		return "Not reported"
	}
	return strconv.FormatFloat(*count, 'f', -1, 64)
}

// FormatReset describes the reset relative to the reading's timestamp. An
// overdue provider timestamp is not a runway estimate of zero.
func FormatReset(reset *time.Time, now time.Time) string {
	if reset == nil {
		return "Not reported"
	}
	if !reset.After(now) {
		return "Passed · awaiting API update"
	}
	return "in " + formatDuration(reset.Sub(now), false)
}

// FormatRunway renders the local estimate without implying exactness.
func FormatRunway(runway domain.RunwayEstimate, key domain.QuotaKey) string {
	switch runway.Status {
	case "depleted":
		return "Depleted"
	case "beyond_reset":
		return "Beyond this reset"
	case "no_consumption":
		return "No consumption observed"
	case "finite":
		if runway.Seconds == nil || math.IsNaN(*runway.Seconds) || math.IsInf(*runway.Seconds, 0) {
			return "Unavailable · non finite estimate"
		}
		return formatDuration(time.Duration(*runway.Seconds*float64(time.Second)), true)
	default:
		reason := strings.ReplaceAll(runway.Reason, "_", " ")
		if reason == "" {
			reason = "insufficient samples"
		}
		return "Unavailable · " + reason
	}
}

// FormatBurn converts the canonical ratio/second burn for the panel's period.
func FormatBurn(runway domain.RunwayEstimate, key domain.QuotaKey) string {
	if runway.BurnRatioPerSecond == nil || math.IsNaN(*runway.BurnRatioPerSecond) || math.IsInf(*runway.BurnRatioPerSecond, 0) {
		return "Not reported"
	}
	factor := 8640000.0
	unit := "%/day"
	if key == domain.QuotaLimit5h {
		factor, unit = 360000, "%/h"
	}
	value := *runway.BurnRatioPerSecond * factor
	if value >= 10 {
		return fmt.Sprintf("%.1f%s", value, unit)
	}
	return fmt.Sprintf("%.2f%s", value, unit)
}

// FormatExhaustion renders an approximate UTC exhaustion timestamp.
func FormatExhaustion(runway domain.RunwayEstimate) string {
	if runway.Status != "finite" || runway.ExhaustedAt == nil {
		return "Not applicable"
	}
	return "≈ " + runway.ExhaustedAt.UTC().Format("15:04:05 UTC")
}

// FormatAge renders coarse cache freshness without implying a new reading.
func FormatAge(age time.Duration) string {
	if age < time.Minute {
		return "just now"
	}
	minutes := int(age / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		if rest := minutes % 60; rest != 0 {
			return fmt.Sprintf("%dh %dm ago", hours, rest)
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	if rest := hours % 24; rest != 0 {
		return fmt.Sprintf("%dd %dh ago", days, rest)
	}
	return fmt.Sprintf("%dd ago", days)
}

// FreshnessCaption labels live and cached data distinctly.
func FreshnessCaption(snapshot domain.Snapshot, cached bool, now time.Time) string {
	if !cached {
		return "Live · " + snapshot.GeneratedAt.UTC().Format("15:04:05 UTC")
	}
	return "Cached data · updated " + FormatAge(now.Sub(snapshot.GeneratedAt))
}

func formatDuration(d time.Duration, round bool) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	if round && d%time.Minute >= 30*time.Second {
		minutes++
	}
	if minutes < 1 {
		return "<1m"
	}
	hours := minutes / 60
	minutes %= 60
	if hours < 48 {
		if hours == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours %= 24
	return fmt.Sprintf("%dd %dh", days, hours)
}
