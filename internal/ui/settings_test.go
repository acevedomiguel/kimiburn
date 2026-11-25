package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func blankBackground(width, height int) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	return strings.Join(lines, "\n")
}

func TestFormatIntervalOption(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "off",
		10 * time.Second: "10s",
		30 * time.Second: "30s",
		time.Minute:      "1m",
		5 * time.Minute:  "5m",
		10 * time.Minute: "10m",
	}
	for d, want := range cases {
		if got := FormatIntervalOption(d); got != want {
			t.Errorf("FormatIntervalOption(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestSettingsContentShowsOptionsSelectionAndTokenRow(t *testing.T) {
	opts := SettingsOptions{
		Width: 80, Height: 24, Styles: NewStylesFor(false, true),
		IntervalOptions: DefaultIntervalOptions(),
		IntervalIndex:   2, Interval: time.Minute,
		TokenAvailable: true,
	}
	content := strings.Join(SettingsContent(opts), "\n")
	for _, want := range []string{"Refresh interval", "10s", "30s", "[1m]", "5m", "10m", "Kimi Code token", "Enter to update token"} {
		if !strings.Contains(content, want) {
			t.Errorf("settings content lacks %q:\n%s", want, content)
		}
	}
	// A custom interval shows no bracket and names the running value.
	custom := SettingsContent(SettingsOptions{
		Width: 80, Height: 24, Styles: NewStylesFor(false, true),
		IntervalOptions: DefaultIntervalOptions(), IntervalIndex: -1, Interval: 45 * time.Second,
	})
	intervalRow := custom[1]
	if strings.Contains(intervalRow, "[") || strings.Contains(intervalRow, "]") {
		t.Errorf("a custom interval must not bracket an option: %q", intervalRow)
	}
	if !strings.Contains(strings.Join(custom, "\n"), "45s") {
		t.Errorf("a custom interval should be named:\n%s", strings.Join(custom, "\n"))
	}
}

func TestSettingsContentMarksFocusAndHidesToken(t *testing.T) {
	editing := strings.Join(SettingsContent(SettingsOptions{
		Width: 80, Height: 24, Styles: NewStylesFor(false, true),
		IntervalOptions: DefaultIntervalOptions(), IntervalIndex: 1, Interval: 30 * time.Second,
		TokenAvailable: true, Focus: 1, Editing: true, TokenLength: 5,
	}), "\n")
	if !strings.Contains(editing, "•••••") {
		t.Errorf("an editing token should render a hidden placeholder:\n%s", editing)
	}
	if strings.Contains(editing, "secret") {
		t.Fatalf("the token value must never appear:\n%s", editing)
	}
}

func TestSettingsOverlayKeepsExactGeometryAndNoColorPurity(t *testing.T) {
	states := []SettingsOptions{
		{IntervalOptions: DefaultIntervalOptions(), IntervalIndex: 0, Interval: 10 * time.Second, TokenAvailable: true, Focus: 0},
		{IntervalOptions: DefaultIntervalOptions(), IntervalIndex: -1, Interval: 45 * time.Second, TokenAvailable: true, Focus: 1, Editing: true, TokenLength: 24, Message: "Checking the token…"},
		{IntervalOptions: DefaultIntervalOptions(), IntervalIndex: 4, Interval: 10 * time.Minute, TokenAvailable: false, EnvToken: true, Message: "Not saved: a very long redacted reason that must wrap without overflowing", MessageIsError: true},
	}
	for _, size := range []struct{ w, h int }{{20, 10}, {40, 12}, {60, 16}, {80, 24}, {120, 40}} {
		for _, noColor := range []bool{false, true} {
			st := NewStylesFor(!noColor, true)
			for i, state := range states {
				state.Width, state.Height, state.Styles = size.w, size.h, st
				frame := SettingsOverlay(blankBackground(size.w, size.h), state)
				if noColor && strings.Contains(frame, "\x1b[") {
					t.Errorf("%dx%d state %d no-color settings gained ANSI", size.w, size.h, i)
				}
				lines := strings.Split(frame, "\n")
				if len(lines) != size.h {
					t.Errorf("%dx%d state %d: %d rows", size.w, size.h, i, len(lines))
				}
				for row, line := range lines {
					if got := lipgloss.Width(line); got != size.w {
						t.Errorf("%dx%d state %d row %d width=%d", size.w, size.h, i, row, got)
					}
				}
				if size.w >= 40 && !strings.Contains(frame, "Settings") {
					t.Errorf("%dx%d state %d settings overlay lacks its title", size.w, size.h, i)
				}
			}
		}
	}
}
