package ui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// HelpContent is the Help document: what the figures mean, the key bindings
// rendered by the help bubble from the same KeyMap the handlers use, and the
// current warnings.
func HelpContent(st Styles, width int, warnings []string) []string {
	textWidth := maxInt(width, 20)
	lines := []string{
		st.Title.Render("What the panels mean"),
		"Monthly membership is the shared membership quota. Monthly Code allocation is included in that membership, not an additional allowance.",
		"Runway is estimated locally from retained readings and bounded by reset time. It is approximate, not a server-provided prediction.",
		"Finite forecasts stop strictly before reset; Beyond reset has no timestamp.",
		"No consumption means burn was zero/negative; Depleted means 100% used.",
		"Unavailable · insufficient samples needs 5 samples over at least 120 seconds.",
		"A reset or ratio decrease clears only that quota's local history.",
		"The timeline runs from now to the reset: ✕ marks the estimated depletion, ✓ means the quota lasts past the reset.",
		"Usage is a sparkline of the retained local readings of the current reset period.",
		"",
		st.Title.Render("Keys"),
	}
	h := NewHelp(st, textWidth)
	h.ShowAll = true
	lines = append(lines, strings.Split(h.View(NewKeyMap()), "\n")...)
	lines = append(lines, "", st.Title.Render("Warnings"))
	if len(warnings) == 0 {
		lines = append(lines, "None")
	}
	for _, warning := range warnings {
		lines = append(lines, st.Warning.Render(warning))
	}

	var out []string
	for _, line := range lines {
		out = append(out, strings.Split(lipgloss.Wrap(line, textWidth, " "), "\n")...)
	}
	return out
}

// HelpTextWidth is the wrap width of the Help document for a terminal width.
func HelpTextWidth(width int) int {
	_, _, textWidth := helpGeometry(width, 0)
	return textWidth
}

// helpGeometry sizes the popup for a terminal: at most 84 columns wide and
// leaving the title row and footer row of the dashboard visible.
func helpGeometry(width, height int) (popupWidth, popupHeight, textWidth int) {
	popupWidth = minInt(84, width-4)
	popupHeight = maxInt(height-4, 5)
	textWidth = maxInt(popupWidth-4, 1)
	return popupWidth, popupHeight, textWidth
}

// HelpScrollLimit reports the largest useful popup offset for this terminal.
func HelpScrollLimit(st Styles, width, height int, warnings []string) int {
	_, popupHeight, textWidth := helpGeometry(width, height)
	lines := HelpContent(st, textWidth, warnings)
	return maxInt(len(lines)-(popupHeight-2), 0)
}

// HelpOverlay centres a bordered, scrollable Help popup above the dashboard
// frame with the lipgloss compositor. The canvas keeps the terminal size, so
// the dashboard and its pinned footer stay on the same rows underneath.
func HelpOverlay(background string, st Styles, width, height, scroll int, warnings []string) string {
	popupWidth, popupHeight, textWidth := helpGeometry(width, height)
	content := HelpContent(st, textWidth, warnings)
	popupHeight = minInt(popupHeight, len(content)+2)
	vp := viewport.New(viewport.WithWidth(textWidth), viewport.WithHeight(popupHeight-2))
	vp.SetContentLines(content)
	vp.SetYOffset(scroll)

	caption := ""
	if !vp.AtTop() || !vp.AtBottom() {
		caption = itoa(int(vp.ScrollPercent()*100+0.5)) + "%"
	}
	popup := block(panelWithCaption("KIMIBURN · Help", caption, strings.Split(vp.View(), "\n"), popupWidth, popupHeight, st))

	x := maxInt((width-popupWidth)/2, 0)
	y := maxInt((height-popupHeight)/2, 0)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(popup).X(x).Y(y).Z(1),
	)
	// The canvas trims trailing blanks; pad back so every row spans the
	// terminal exactly like the dashboard frame underneath.
	lines := strings.Split(lipgloss.NewCanvas(width, height).Compose(compositor).Render(), "\n")
	for i, line := range lines {
		lines[i] = padRight(line, width)
	}
	return strings.Join(lines, "\n")
}
