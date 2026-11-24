package app

import (
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/acevedomiguel/kimiburn/internal/domain"
	"github.com/acevedomiguel/kimiburn/internal/ui"
)

// readyDemo returns a demo model that has applied its first synthetic reading.
func readyDemo(t *testing.T, width, height int, noColor bool) Model {
	t.Helper()
	wall := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewModel(Options{Demo: true, Width: width, Height: height, NoColor: noColor, Now: func() time.Time { return wall }})
	updated, cmd := m.Update(m.Init()())
	updated, _ = updated.(Model).Update(cmd())
	return updated.(Model)
}

func TestColoredFramesKeepExactGeometry(t *testing.T) {
	for _, size := range []struct{ w, h int }{{60, 16}, {80, 24}, {100, 40}, {140, 20}} {
		m := readyDemo(t, size.w, size.h, false)
		for _, inFlight := range []bool{false, true} {
			m.inFlight = inFlight
			frames := map[string]string{"dashboard": m.View().Content}
			help, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
			frames["help"] = help.(Model).View().Content
			for name, frame := range frames {
				lines := strings.Split(frame, "\n")
				if len(lines) != size.h {
					t.Errorf("%dx%d %s: %d rows", size.w, size.h, name, len(lines))
				}
				for i, line := range lines {
					if got := lipgloss.Width(line); got != size.w {
						t.Errorf("%dx%d %s row %d width=%d", size.w, size.h, name, i, got)
					}
				}
			}
		}
	}
}

func TestDemoShowsTimelineTrendAndCountdown(t *testing.T) {
	m := readyDemo(t, 120, 40, true)
	view := m.View().Content
	for _, want := range []string{"now ●", "✕", "reset ✓", "Usage ▁", "+5.9% in 30m", "next read 30s", "in 3h 12m"} {
		if !strings.Contains(view, want) {
			t.Errorf("demo view lacks %q:\n%s", want, view)
		}
	}
	// The demo clock advances with the wall clock from the fixture's time, so
	// the countdowns move without a new reading.
	start := m.now()
	if !start.Equal(ui.SyntheticTime()) {
		t.Fatalf("demo clock starts at %s, want %s", start, ui.SyntheticTime())
	}
}

func TestRefreshingShowsSpinnerInFooter(t *testing.T) {
	m := readyDemo(t, 100, 30, true)
	m.inFlight = true
	lines := strings.Split(m.View().Content, "\n")
	if footer := lines[len(lines)-1]; !strings.Contains(footer, "refreshing") || !strings.Contains(footer, "[r] refresh") {
		t.Fatalf("footer=%q", footer)
	}
}

func TestMetersSpringToEachReading(t *testing.T) {
	m := readyDemo(t, 100, 30, true)
	cmd := m.syncMeters()
	if cmd == nil {
		t.Fatal("first reading did not start the meter animation")
	}
	if got := m.shownMeters()[domain.QuotaLimit5h]; got != 0 {
		t.Fatalf("a new meter starts at %.3f, want 0", got)
	}
	for i := 0; i < 600; i++ {
		if m.stepMeters(meterFrameMsg{gen: m.meterGen}) == nil {
			break
		}
	}
	if got := m.shownMeters()[domain.QuotaLimit5h]; got < 0.279 || got > 0.281 {
		t.Fatalf("5-hour meter settled at %.3f, want 0.28", got)
	}
	if m.stepMeters(meterFrameMsg{gen: m.meterGen - 1}) != nil {
		t.Fatal("a superseded animation frame kept ticking")
	}
}

func TestWindowTitleAndTerminalProgress(t *testing.T) {
	m := readyDemo(t, 100, 30, true)
	m.termProgress = true
	view := m.View()
	if view.WindowTitle != "kimiburn · 5h 28.0% left · runway 2h 20m" {
		t.Fatalf("title=%q", view.WindowTitle)
	}
	if view.ProgressBar == nil || view.ProgressBar.State != tea.ProgressBarWarning || view.ProgressBar.Value != 28 {
		t.Fatalf("progress=%+v", view.ProgressBar)
	}
	m.termProgress = false
	if m.View().ProgressBar != nil {
		t.Fatal("terminal progress must stay off unless the terminal supports it")
	}
	loading := NewModel(Options{})
	if title := loading.View().WindowTitle; title != "kimiburn" {
		t.Fatalf("loading title=%q", title)
	}
}

func TestBackgroundColorSelectsPalette(t *testing.T) {
	m := readyDemo(t, 100, 30, false)
	updated, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	if updated.(Model).styles.Dark {
		t.Fatal("a white background kept the dark palette")
	}
	updated, _ = updated.Update(tea.BackgroundColorMsg{Color: color.Black})
	if !updated.(Model).styles.Dark {
		t.Fatal("a black background did not select the dark palette")
	}
	pinned := NewModel(Options{Theme: "light", Snapshot: ui.SyntheticSnapshot()})
	updated, _ = pinned.Update(tea.BackgroundColorMsg{Color: color.Black})
	if updated.(Model).styles.Dark {
		t.Fatal("theme: light was overridden by the terminal background")
	}
	plain := readyDemo(t, 100, 30, true)
	updated, _ = plain.Update(tea.BackgroundColorMsg{Color: color.White})
	if strings.Contains(updated.(Model).View().Content, "\x1b[") {
		t.Fatal("--no-color frame gained ANSI after the background query")
	}
}

func TestMouseWheelScrollsDashboardAndHelp(t *testing.T) {
	m := readyDemo(t, 80, 16, true)
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if got := updated.(Model).scroll; got != 3 {
		t.Fatalf("wheel down scroll=%d, want 3", got)
	}
	updated, _ = updated.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if got := updated.(Model).scroll; got != 0 {
		t.Fatalf("wheel up scroll=%d, want 0", got)
	}
	if mode := m.View().MouseMode; mode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode=%v", mode)
	}
}

func TestHelpOverlayKeepsDashboardBehind(t *testing.T) {
	m := readyDemo(t, 120, 40, true)
	m = m.openHelp()
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"KIMIBURN · Help", "SYNTHETIC DATA", "What the panels mean", "[esc/?/q] close"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay lacks %q:\n%s", want, view)
		}
	}
}
