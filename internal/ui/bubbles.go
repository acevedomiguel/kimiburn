package ui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// NewHelp builds the help bubble used for the footer and the Help overlay.
func NewHelp(s Styles, width int) help.Model {
	h := help.New()
	h.SetWidth(width)
	h.ShortSeparator = " · "
	h.FullSeparator = "    "
	if !s.Enabled {
		empty := lipgloss.NewStyle()
		h.Styles = help.Styles{
			Ellipsis: empty, ShortKey: empty, ShortDesc: empty, ShortSeparator: empty,
			FullKey: empty, FullDesc: empty, FullSeparator: empty,
		}
		return h
	}
	h.Styles = help.Styles{
		Ellipsis:       s.Muted,
		ShortKey:       s.Key,
		ShortDesc:      s.Muted,
		ShortSeparator: s.Muted,
		FullKey:        s.Key,
		FullDesc:       s.Muted,
		FullSeparator:  s.Muted,
	}
	return h
}

// NewSpinner builds the refresh indicator.
func NewSpinner(s Styles) spinner.Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	if s.Enabled {
		sp.Style = s.Accent
	}
	return sp
}

// NewMeterAnimator builds a progress bubble used only for its spring physics:
// the app feeds it target percentages and reads back the displayed one, and
// the dashboard draws the bar itself. Rendering therefore stays pure.
func NewMeterAnimator() progress.Model {
	return progress.New(progress.WithoutPercentage(), progress.WithSpringOptions(18, 1))
}

// meterBar renders a remaining-quota bar of width cells with the progress
// bubble, coloured on the red-amber-green gauge by the remaining fraction.
// Without colour it falls back to plain block glyphs.
func meterBar(shown, remaining float64, width int, st Styles) string {
	if width < 1 {
		return ""
	}
	if !st.Enabled {
		fill := int(clamp01(shown)*float64(width) + 0.5)
		return strings.Repeat("▇", fill) + strings.Repeat("░", width-fill)
	}
	fg := st.gaugeColor(remaining)
	bar := progress.New(
		progress.WithoutPercentage(),
		progress.WithWidth(width),
		progress.WithFillCharacters('▇', '░'),
		progress.WithColorFunc(func(float64, float64) color.Color { return fg }),
	)
	bar.EmptyColor = st.MeterEmpty
	return bar.ViewAs(shown)
}

func clamp01(v float64) float64 {
	if v != v || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
