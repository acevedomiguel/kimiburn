package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Styles bundles every Lip Gloss style used by the dashboard.
type Styles struct {
	Enabled bool
	// Dark records which terminal background the palette was resolved for.
	Dark bool

	Base     lipgloss.Style
	Title    lipgloss.Style
	Muted    lipgloss.Style
	Accent   lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	Error    lipgloss.Style
	Card     lipgloss.Style
	CardHead lipgloss.Style
	Label    lipgloss.Style
	Value    lipgloss.Style
	Banner   lipgloss.Style
	Help     lipgloss.Style
	Key      lipgloss.Style
	Bar      lipgloss.Style

	StatusBar lipgloss.Style

	// Gauge is the remaining-quota scale, from empty (index 0) to full (last).
	// It is nil when colour is disabled.
	Gauge []color.Color
	// MeterEmpty colours the unfilled part of a meter.
	MeterEmpty color.Color
}

// gaugeSteps is the resolution of the remaining-quota colour scale.
const gaugeSteps = 101

// NewStyles builds the style set for a dark terminal background. When enabled
// is false every style renders plain text so --no-color and NO_COLOR produce
// identical output.
func NewStyles(enabled bool) Styles {
	return NewStylesFor(enabled, true)
}

// NewStylesFor builds the style set for the detected terminal background, so
// the status bar and gauges stay legible on light themes as well as dark ones.
func NewStylesFor(enabled, dark bool) Styles {
	pick := lipgloss.LightDark(dark)
	var (
		accent  = pick(lipgloss.Color("#00819A"), lipgloss.Color("#4FC1D6"))
		muted   = pick(lipgloss.Color("#767676"), lipgloss.Color("#8A8A8A"))
		success = pick(lipgloss.Color("#2E8B3A"), lipgloss.Color("#5FD787"))
		warning = pick(lipgloss.Color("#A66A00"), lipgloss.Color("#FFAF00"))
		danger  = pick(lipgloss.Color("#C62828"), lipgloss.Color("#FF5F5F"))
		empty   = pick(lipgloss.Color("#D0D0D0"), lipgloss.Color("#3A3A3A"))
		bar     = pick(lipgloss.Color("#E4E4E4"), lipgloss.Color("#262626"))
	)

	build := func(fg color.Color, bold bool) lipgloss.Style {
		if !enabled {
			return lipgloss.NewStyle()
		}
		s := lipgloss.NewStyle()
		if fg != nil {
			s = s.Foreground(fg)
		}
		if bold {
			s = s.Bold(true)
		}
		return s
	}

	st := Styles{
		Enabled:   enabled,
		Dark:      dark,
		Base:      build(nil, false),
		Title:     build(accent, true),
		Muted:     build(muted, false),
		Accent:    build(accent, false),
		Success:   build(success, false),
		Warning:   build(warning, false),
		Error:     build(danger, false),
		Card:      build(nil, false),
		CardHead:  build(accent, true),
		Label:     build(muted, false),
		Value:     build(nil, true),
		Banner:    build(danger, false),
		Help:      build(muted, false),
		Key:       build(accent, false),
		Bar:       build(success, false),
		StatusBar: lipgloss.NewStyle().Padding(0, 1),
	}
	if enabled {
		st.StatusBar = st.StatusBar.Background(bar)
		st.Gauge = lipgloss.Blend1D(gaugeSteps, danger, warning, success)
		st.MeterEmpty = empty
	}
	return st
}

// gaugeColor maps a remaining fraction onto the red-amber-green scale.
func (s Styles) gaugeColor(remaining float64) color.Color {
	if len(s.Gauge) == 0 {
		return nil
	}
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 1 {
		remaining = 1
	}
	return s.Gauge[int(remaining*float64(len(s.Gauge)-1)+0.5)]
}

// gaugeStyle is a foreground style in the gauge colour for remaining.
func (s Styles) gaugeStyle(remaining float64) lipgloss.Style {
	if c := s.gaugeColor(remaining); c != nil {
		return lipgloss.NewStyle().Foreground(c)
	}
	return lipgloss.NewStyle()
}
