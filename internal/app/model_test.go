package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/acevedomiguel/kimiburn/internal/ui"
)

func TestInitStatesAndAuthSuspension(t *testing.T) {
	now := mustNow()
	m := NewModel(Options{Now: func() time.Time { return now }})
	m = stateInit(m, now)
	if m.state != ViewSetup || m.inFlight {
		t.Fatalf("missing token state=%v inFlight=%v", m.state, m.inFlight)
	}
	m = NewModel(Options{Demo: true, Configured: func() (bool, error) { return true, nil }})
	m = stateInit(m, now)
	if m.state != ViewLoading {
		t.Fatalf("demo init state=%v", m.state)
	}

	m = NewModel(Options{Now: func() time.Time { return now }})
	m.generation, m.inFlight, m.manualSuccess = 7, true, true
	m = m.applyRefresh(RefreshResult{Generation: 7, AuthFailure: true})
	if m.state != ViewSetup || m.inFlight || !m.suspended || m.manualSuccess {
		t.Fatalf("auth state=%+v", m)
	}
	m = m.applyRefresh(RefreshResult{Generation: 7, Snapshot: ui.SyntheticSnapshot()})
	if !m.suspended {
		t.Fatal("automatic success must not clear an auth suspension")
	}
	m.manual = 1
	m = m.applyRefresh(RefreshResult{Generation: 7, Manual: true, Snapshot: ui.SyntheticSnapshot()})
	if m.suspended {
		t.Fatal("manual success did not clear auth suspension")
	}
}

func TestResizeAndWarmCacheControls(t *testing.T) {
	m := NewModel(Options{Now: func() time.Time { return mustNow() }, Width: 80, Height: 24})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)
	if m.width != 100 || m.height != 40 {
		t.Fatalf("size=%dx%d", m.width, m.height)
	}
	m.snapshot, m.state, m.cached = ui.SyntheticSnapshot(), ViewReady, true
	m.lastAttempt = mustNow()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd != nil || m.inFlight || m.nextAllowed != m.lastAttempt.Add(30*time.Second) {
		t.Fatalf("throttled cache attempt=%+v", m)
	}
	if m.scroll != 0 {
		t.Fatalf("scroll=%d", m.scroll)
	}
}

func TestCancellationAndGenerationSupersession(t *testing.T) {
	cancelled := false
	m := NewModel(Options{Now: func() time.Time { return mustNow() }, Cancel: func() { cancelled = true }})
	m.inFlight = true
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("quit did not return tea.Quit")
	}
	if !cancelled {
		t.Fatal("quit did not cancel in-flight refresh")
	}

	m = NewModel(Options{Now: func() time.Time { return mustNow() }})
	m.generation, m.manual = 4, 5
	stale := m.applyRefresh(RefreshResult{Generation: 3, Manual: true, Snapshot: ui.SyntheticSnapshot()})
	if stale.state == ViewReady || stale.inFlight {
		t.Fatalf("stale result replaced state: %+v", stale)
	}
	cancelled = false
	m.cancelAttempt = func() { cancelled = true }
	m.inFlight = true
	m = m.applyRefresh(RefreshResult{Generation: 4, Manual: true})
	if !cancelled || m.inFlight {
		t.Fatal("matching completion did not cancel/close generation")
	}
}

func TestHelpAndScrollRestore(t *testing.T) {
	now := mustNow()
	snapshot := ui.SyntheticUnavailableSnapshot()
	m := NewModel(Options{Now: func() time.Time { return now }, Width: 100, Height: 40, Snapshot: snapshot})
	m.scroll = 9
	m = m.openHelp()
	if m.state != ViewHelp || m.scroll != 0 {
		t.Fatalf("help state=%v scroll=%d", m.state, m.scroll)
	}
	m.maxScroll()
	m.scroll = 3
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.state != ViewReady || m.scroll != 9 {
		t.Fatalf("restore state=%v scroll=%d", m.state, m.scroll)
	}
	view := m.View()
	if view.AltScreen != true {
		t.Fatal("interactive view must use alt screen")
	}
	if !strings.Contains(view.Content, "[r] refresh") {
		t.Fatal("footer missing refresh binding")
	}
	for _, forbidden := range []string{"[tab]", "[←→]", "tokens"} {
		if strings.Contains(strings.ToLower(view.Content), forbidden) {
			t.Fatalf("forbidden binding/concept %q in view", forbidden)
		}
	}
}

func TestHelpExplainsRunwayStatesWithoutConfidenceLabels(t *testing.T) {
	m := NewModel(Options{Width: 100, Height: 40, Snapshot: ui.SyntheticSnapshot()})
	help := strings.Join(m.helpLines(), "\n")
	for _, phrase := range []string{"Monthly Code allocation", "not a server-provided prediction", "Beyond reset", "No consumption", "Depleted", "insufficient samples", "reset"} {
		if !strings.Contains(help, phrase) {
			t.Fatalf("help lacks %q:\n%s", phrase, help)
		}
	}
	for _, unwanted := range []string{"confidence", "Kimi Code monthly"} {
		if strings.Contains(help, unwanted) {
			t.Fatalf("help contains unwanted %q:\n%s", unwanted, help)
		}
	}
}

func TestFatalAndSetupRenderWithoutData(t *testing.T) {
	m := NewModel(Options{Width: 80, Height: 24})
	m.state, m.errText = ViewFatal, "request failed"
	if got := m.View().Content; !strings.Contains(got, "Refresh failed") || !strings.Contains(got, "request failed") {
		t.Fatalf("fatal view: %s", got)
	}
	m.state = ViewSetup
	if got := m.View().Content; !strings.Contains(got, "Token setup required") {
		t.Fatalf("setup view: %s", got)
	}
}

func TestDemoManualRefreshReloadsSyntheticDataWithoutWarning(t *testing.T) {
	m := NewModel(Options{Demo: true, Width: 100, Height: 40})
	updated, cmd := m.Update(m.Init()())
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("demo refresh returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != ViewReady || m.warning != "" {
		t.Fatalf("demo refresh state=%v warning=%q", m.state, m.warning)
	}
	if strings.Contains(m.View().Content, "refresh is not configured") {
		t.Fatal("demo refresh displayed a live-provider configuration warning")
	}
}

func TestModelEnablesColorByDefaultAndHonorsNoColor(t *testing.T) {
	colored := NewModel(Options{Width: 100, Height: 40, Snapshot: ui.SyntheticSnapshot()}).View().Content
	if !strings.Contains(colored, "\x1b[") {
		t.Fatal("default dashboard contains no ANSI color styling")
	}
	plain := NewModel(Options{Width: 100, Height: 40, Snapshot: ui.SyntheticSnapshot(), NoColor: true}).View().Content
	if strings.Contains(plain, "\x1b[") {
		t.Fatal("--no-color dashboard contains ANSI styling")
	}
}
