package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate cuts plain text to the terminal cell width.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		var b strings.Builder
		used := 0
		for _, r := range s {
			rw := lipgloss.Width(string(r))
			if used+rw > width {
				break
			}
			b.WriteRune(r)
			used += rw
		}
		return b.String()
	}
	target := width - 3
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > target {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wrapPlain hard-wraps JSON or diagnostic text without splitting escape
// sequences; callers use it only for syntax-neutral plain text.
func wrapPlain(s string, width int) []string {
	if width < 1 {
		return []string{""}
	}
	var lines []string
	for _, logical := range strings.Split(s, "\n") {
		if logical == "" {
			lines = append(lines, "")
			continue
		}
		var b strings.Builder
		used := 0
		for _, r := range logical {
			rw := lipgloss.Width(string(r))
			if used > 0 && used+rw > width {
				lines = append(lines, b.String())
				b.Reset()
				used = 0
			}
			b.WriteRune(r)
			used += rw
		}
		lines = append(lines, b.String())
	}
	return lines
}
