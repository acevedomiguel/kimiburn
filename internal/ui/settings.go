package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// DefaultIntervalOptions are the automatic-refresh intervals the Settings
// overlay offers, in ascending order.
func DefaultIntervalOptions() []time.Duration {
	return []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, 5 * time.Minute, 10 * time.Minute}
}

// SettingsOptions describes the Settings overlay's contents. Everything is
// plain data so rendering stays pure and testable.
type SettingsOptions struct {
	Width, Height int
	Styles        Styles

	// IntervalOptions are the selectable intervals in display order.
	// IntervalIndex selects one, or -1 when the running interval is a custom
	// value that is not a menu option.
	IntervalOptions []time.Duration
	IntervalIndex   int
	Interval        time.Duration
	// IntervalPinned is true when a flag or environment variable outranks the
	// saved file, so a change applies now but not on the next start.
	IntervalPinned bool
	// IntervalSaved is true when a change is persisted to a configuration file.
	IntervalSaved bool

	// Focus is 0 for the interval row and 1 for the token row.
	Focus int

	TokenAvailable bool
	EnvToken       bool
	Editing        bool
	TokenLength    int
	Saving         bool

	Message        string
	MessageIsError bool
}

// SettingsContent is the Settings overlay body as styled lines.
func SettingsContent(opts SettingsOptions) []string {
	st := opts.Styles
	lines := []string{
		st.Title.Render("Refresh interval"),
		intervalRow(opts),
		st.Muted.Render(intervalNote(opts)),
		"",
		st.Title.Render("Kimi Code token"),
		tokenRow(opts),
	}
	if note := tokenNote(opts); note != "" {
		lines = append(lines, st.Muted.Render(note))
	}
	if opts.Message != "" {
		style := st.Success
		if opts.MessageIsError {
			style = st.Error
		}
		lines = append(lines, "", style.Render(opts.Message))
	}
	return lines
}

// intervalRow draws the interval options, bracketing the selected one and
// marking the row when it holds focus.
func intervalRow(opts SettingsOptions) string {
	parts := make([]string, 0, len(opts.IntervalOptions))
	for i, d := range opts.IntervalOptions {
		label := FormatIntervalOption(d)
		if i == opts.IntervalIndex {
			label = "[" + label + "]"
		} else {
			label = " " + label + " "
		}
		parts = append(parts, label)
	}
	row := strings.Join(parts, " ")
	if row == "" {
		row = "none"
	}
	return focusPrefix(opts.Focus == 0) + row
}

// intervalNote explains the current interval selection: a custom value, an
// externally pinned one, a session-only one, or one saved to disk.
func intervalNote(opts SettingsOptions) string {
	switch {
	case opts.IntervalIndex < 0:
		return "current: " + FormatIntervalOption(opts.Interval) + " (custom)"
	case opts.IntervalPinned:
		return "--refresh or KIMIBURN_REFRESH_INTERVAL overrides the saved file on restart"
	case !opts.IntervalSaved:
		return "applies to this session only"
	default:
		return "saved to the configuration file"
	}
}

// tokenRow draws the token row: a prompt to begin, the hidden input, or a
// progress marker while the token is checked.
func tokenRow(opts SettingsOptions) string {
	prefix := focusPrefix(opts.Focus == 1)
	st := opts.Styles
	switch {
	case !opts.TokenAvailable:
		return prefix + st.Muted.Render("[ Update token · unavailable ]")
	case opts.Editing:
		return prefix + "New Kimi Code token: " + strings.Repeat("•", opts.TokenLength) + "█"
	case opts.Saving:
		return prefix + st.Muted.Render("[ checking the token… ]")
	default:
		return prefix + st.Key.Render("[ Enter to update token ]")
	}
}

// tokenNote explains the token row's current state.
func tokenNote(opts SettingsOptions) string {
	switch {
	case opts.EnvToken:
		return "KIMIBURN_TOKEN supplies the token; unset it to store one here."
	case !opts.TokenAvailable:
		return "Token updates are unavailable in this mode."
	case opts.Editing:
		return "The token is hidden. Enter validates and saves it; Esc cancels."
	default:
		return "Replacing the token checks it with one usage read before saving."
	}
}

func focusPrefix(focused bool) string {
	if focused {
		return "› "
	}
	return "  "
}

// settingsHelp adapts a group of bindings to the help bubble.
type settingsHelp struct{ groups [][]key.Binding }

func (h settingsHelp) ShortHelp() []key.Binding {
	if len(h.groups) == 0 {
		return nil
	}
	return h.groups[0]
}
func (h settingsHelp) FullHelp() [][]key.Binding { return h.groups }

// SettingsOverlay centres the Settings panel above the dashboard frame, sized
// to the terminal exactly like the Help overlay.
func SettingsOverlay(background string, opts SettingsOptions) string {
	st := opts.Styles
	width, height := maxInt(opts.Width, 1), maxInt(opts.Height, 1)
	popupWidth := minInt(84, maxInt(width-4, 4))
	if popupWidth > width {
		popupWidth = width
	}
	textWidth := maxInt(popupWidth-4, 1)

	content := SettingsContent(opts)
	var wrapped []string
	for _, line := range content {
		wrapped = append(wrapped, strings.Split(lipgloss.Wrap(line, textWidth, " "), "\n")...)
	}
	keys := NewSettingsKeyMap()
	footer := NewHelp(st, textWidth).View(settingsHelp{groups: keys.SettingsFooter(opts.Editing)})
	wrapped = append(wrapped, "", footer)

	panelHeight := len(wrapped) + 2
	if panelHeight > height {
		panelHeight = height
	}
	popup := block(panelWithCaption("KIMIBURN · Settings", "", wrapped, popupWidth, panelHeight, st))

	x := maxInt((width-popupWidth)/2, 0)
	y := maxInt((height-panelHeight)/2, 0)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(popup).X(x).Y(y).Z(1),
	)
	lines := strings.Split(lipgloss.NewCanvas(width, height).Compose(compositor).Render(), "\n")
	for i, line := range lines {
		lines[i] = padRight(line, width)
	}
	return strings.Join(lines, "\n")
}

// FormatIntervalOption renders an automatic-refresh interval the way the
// Settings menu spells it: whole minutes as "Nm", whole seconds as "Ns", and a
// disabled cadence as "off".
func FormatIntervalOption(d time.Duration) string {
	switch {
	case d <= 0:
		return "off"
	case d%time.Minute == 0:
		return itoa(int(d/time.Minute)) + "m"
	}
	return itoa(int(d/time.Second)) + "s"
}
