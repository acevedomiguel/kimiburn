package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/acevedomiguel/kimiburn/internal/ui"
)

// settingsModel returns a ready dashboard with the Settings overlay wired.
func settingsModel(opts Options) Model {
	if opts.Now == nil {
		opts.Now = func() time.Time { return mustNow() }
	}
	if opts.Width == 0 {
		opts.Width = 100
	}
	if opts.Height == 0 {
		opts.Height = 40
	}
	if opts.Snapshot.GeneratedAt.IsZero() {
		opts.Snapshot = ui.SyntheticSnapshot()
	}
	if len(opts.IntervalOptions) == 0 {
		opts.IntervalOptions = DefaultIntervalOptions()
	}
	if opts.Interval == 0 {
		opts.Interval, opts.IntervalSet = 30*time.Second, true
	}
	return NewModel(opts)
}

func press(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func typeKeys(m Model, text string) Model {
	for _, r := range text {
		m, _ = press(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// paste delivers a bracketed-paste message the way a terminal does, exercising
// the same path as a real Cmd+V.
func paste(m Model, text string) Model {
	updated, _ := m.Update(tea.PasteMsg{Content: text})
	return updated.(Model)
}

func openSettingsForTest(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.state != ViewSettings {
		t.Fatalf("state = %v, want ViewSettings", m.state)
	}
	return m
}

func TestSettingsOverlayOpensAndClosesRestoringState(t *testing.T) {
	m := settingsModel(Options{})
	m.scroll = 3
	m = openSettingsForTest(t, m)
	if m.savedState != ViewReady || m.savedScroll != 3 {
		t.Fatalf("saved state=%v scroll=%d", m.savedState, m.savedScroll)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state != ViewReady || m.scroll != 3 {
		t.Fatalf("close state=%v scroll=%d, want the dashboard at scroll 3", m.state, m.scroll)
	}

	// q and s close the overlay too, matching the Help overlay's q.
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.state != ViewReady {
		t.Fatalf("q did not close Settings: state=%v", m.state)
	}
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.state != ViewReady {
		t.Fatalf("s did not close Settings: state=%v", m.state)
	}
}

func TestSettingsTokenEditingTreatsLettersAsContent(t *testing.T) {
	m := settingsModel(Options{
		ValidateToken: func(context.Context, string) error { return nil },
		SaveToken:     func(context.Context, string) error { return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	// s and q are overlay commands outside editing, but token content inside it.
	m = typeKeys(m, "sq")
	if m.state != ViewSettings || !m.tokenEditing {
		t.Fatalf("state=%v editing=%v, want the overlay still editing", m.state, m.tokenEditing)
	}
	if m.tokenInput != "sq" {
		t.Fatalf("tokenInput = %q, want \"sq\"", m.tokenInput)
	}
}

func TestSettingsIntervalSelectionPersistsAndRearms(t *testing.T) {
	var saved []time.Duration
	m := settingsModel(Options{SetInterval: func(d time.Duration) error {
		saved = append(saved, d)
		return nil
	}})
	m = openSettingsForTest(t, m)
	if m.settingsIndex != 1 {
		t.Fatalf("initial index = %d, want 1 for 30s", m.settingsIndex)
	}

	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.interval != time.Minute {
		t.Fatalf("interval = %v, want 1m", m.interval)
	}
	if len(saved) != 1 || saved[0] != time.Minute {
		t.Fatalf("saved = %v, want one 1m write", saved)
	}
	if cmd == nil {
		t.Fatal("changing the interval did not re-arm the timer")
	}
	if m.settingsErr || m.settingsMsg == "" {
		t.Fatalf("message=%q err=%v", m.settingsMsg, m.settingsErr)
	}

	// A number key selects directly, and left clamps at the smallest option.
	m, _ = press(m, tea.KeyPressMsg{Code: '4', Text: "4"})
	if m.interval != 5*time.Minute {
		t.Fatalf("number key interval = %v, want 5m", m.interval)
	}
	for i := 0; i < 8; i++ {
		m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	if m.interval != 10*time.Second {
		t.Fatalf("clamped interval = %v, want 10s", m.interval)
	}
}

func TestSettingsIntervalWithoutPersisterIsSessionOnly(t *testing.T) {
	m := settingsModel(Options{})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.interval != time.Minute {
		t.Fatalf("interval = %v, want 1m", m.interval)
	}
	if !strings.Contains(m.settingsMsg, "session") {
		t.Fatalf("message = %q, want a session-only note", m.settingsMsg)
	}
}

func TestSettingsIntervalSaveFailureStillAppliesForTheSession(t *testing.T) {
	m := settingsModel(Options{SetInterval: func(time.Duration) error {
		return errors.New("config: parse /tmp/config.yaml: boom")
	}})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.interval != time.Minute {
		t.Fatalf("interval = %v, want the session change to stand", m.interval)
	}
	if !m.settingsErr || !strings.Contains(m.settingsMsg, "could not save") {
		t.Fatalf("message=%q err=%v", m.settingsMsg, m.settingsErr)
	}
}

func TestSettingsIntervalChangeSupersedesTheRunningTimer(t *testing.T) {
	m := settingsModel(Options{})
	m = openSettingsForTest(t, m)
	stale := m.tickGen

	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd == nil {
		t.Fatal("the interval change did not re-arm the timer")
	}
	if m.tickGen == stale {
		t.Fatal("the interval change did not start a new timer chain")
	}
	if updated, next := m.Update(tickMsg{gen: stale}); next != nil {
		t.Fatalf("a superseded tick still started work: %+v", updated)
	}
	if _, next := m.Update(tickMsg{gen: m.tickGen}); next == nil {
		t.Fatal("the current tick did not start an attempt")
	}
}

func TestSettingsTokenEntryIsHiddenValidatedAndSaved(t *testing.T) {
	var validated, stored string
	const token = "km_synthetic_token_value"
	m := settingsModel(Options{
		ValidateToken: func(_ context.Context, got string) error { validated = got; return nil },
		SaveToken:     func(_ context.Context, got string) error { stored = got; return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.settingsFocus != 1 {
		t.Fatalf("focus = %d, want the token row", m.settingsFocus)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.tokenEditing {
		t.Fatal("enter on the token row did not start editing")
	}

	m = typeKeys(m, token)
	if view := m.View().Content; strings.Contains(view, token) {
		t.Fatalf("the typed token appeared in the view:\n%s", view)
	}
	if bullets := strings.Repeat("•", len([]rune(token))); !strings.Contains(m.View().Content, bullets) {
		t.Fatalf("the hidden input did not render %d bullets", len([]rune(token)))
	}

	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting the token returned no command")
	}
	if m.tokenInput != "" {
		t.Fatalf("the token buffer was not cleared: %q", m.tokenInput)
	}
	if !m.tokenSaving {
		t.Fatal("the model should mark the token as being checked")
	}

	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if validated != token || stored != token {
		t.Fatalf("validated=%q stored=%q, want the submitted token", validated, stored)
	}
	if m.tokenSaving || m.settingsErr {
		t.Fatalf("after save: saving=%v err=%v msg=%q", m.tokenSaving, m.settingsErr, m.settingsMsg)
	}
}

func TestSettingsTokenFailureKeepsPreviousTokenAndRedacts(t *testing.T) {
	saved := false
	const token = "km_synthetic_secret_value"
	m := settingsModel(Options{
		ValidateToken: func(_ context.Context, got string) error {
			return errors.New("server rejected " + got)
		},
		SaveToken: func(context.Context, string) error { saved = true; return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = typeKeys(m, token)
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting the token returned no command")
	}
	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if saved {
		t.Fatal("a failed validation must not save the token")
	}
	if !m.settingsErr {
		t.Fatal("a failed validation should be reported as an error")
	}
	if strings.Contains(m.settingsMsg, token) {
		t.Fatalf("the failure message leaked the token: %q", m.settingsMsg)
	}
}

func TestSettingsEnvironmentTokenDisablesTokenUpdate(t *testing.T) {
	m := settingsModel(Options{
		EnvToken:      true,
		ValidateToken: func(context.Context, string) error { return nil },
		SaveToken:     func(context.Context, string) error { return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.tokenEditing {
		t.Fatal("editing must be disabled when KIMIBURN_TOKEN is set")
	}
	if view := m.View().Content; !strings.Contains(view, "KIMIBURN_TOKEN") {
		t.Fatalf("the overlay should explain the environment token:\n%s", view)
	}
}

func TestSettingsTokenSaveClearsSuspensionAndForcesRefresh(t *testing.T) {
	m := settingsModel(Options{
		ValidateToken: func(context.Context, string) error { return nil },
		SaveToken:     func(context.Context, string) error { return nil },
	})
	m.state = ViewSetup
	m = openSettingsForTest(t, m)
	m.suspended = true
	m.generation = 4

	m, cmd := m.applyTokenSaved(tokenSavedMsg{})
	if m.suspended {
		t.Fatal("a saved token did not clear the auth suspension")
	}
	if !m.inFlight {
		t.Fatal("a saved token did not start a refresh")
	}
	if cmd == nil {
		t.Fatal("a saved token returned no command")
	}
	if m.savedState != ViewReady {
		t.Fatalf("savedState = %v, want ViewReady after a token is stored", m.savedState)
	}
}

func TestSettingsTokenPasteAcceptsContentAndHidesIt(t *testing.T) {
	var validated, stored string
	const token = "km_synthetic_pasted_token"
	m := settingsModel(Options{
		ValidateToken: func(_ context.Context, got string) error { validated = got; return nil },
		SaveToken:     func(_ context.Context, got string) error { stored = got; return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.tokenEditing {
		t.Fatal("enter on the token row did not start editing")
	}

	// A paste containing the overlay's own letters must be token content, not
	// commands, and it must never appear in the rendered view.
	m = paste(m, token)
	if m.tokenInput != token {
		t.Fatalf("tokenInput = %q, want the pasted token", m.tokenInput)
	}
	if m.state != ViewSettings || !m.tokenEditing {
		t.Fatalf("state=%v editing=%v, want the overlay still editing", m.state, m.tokenEditing)
	}
	if view := m.View().Content; strings.Contains(view, token) {
		t.Fatalf("the pasted token appeared in the view:\n%s", view)
	}
	if bullets := strings.Repeat("•", len([]rune(token))); !strings.Contains(m.View().Content, bullets) {
		t.Fatalf("the hidden input did not render %d bullets", len([]rune(token)))
	}

	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting the pasted token returned no command")
	}
	if !m.tokenSaving || m.tokenInput != "" {
		t.Fatalf("saving=%v tokenInput=%q", m.tokenSaving, m.tokenInput)
	}
	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if validated != token || stored != token {
		t.Fatalf("validated=%q stored=%q, want the pasted token", validated, stored)
	}
}

func TestSettingsTokenPasteIsIgnoredOutsideEditing(t *testing.T) {
	// On the dashboard a paste changes nothing.
	m := settingsModel(Options{})
	m = paste(m, "km_synthetic_token")
	if m.tokenInput != "" {
		t.Fatalf("a dashboard paste was captured: %q", m.tokenInput)
	}

	// Inside the overlay, before token entry starts, a paste is still ignored.
	m = openSettingsForTest(t, m)
	m = paste(m, "km_synthetic_token")
	if m.tokenEditing || m.tokenInput != "" {
		t.Fatalf("editing=%v tokenInput=%q, want a paste ignored", m.tokenEditing, m.tokenInput)
	}
}

func TestSettingsTokenPasteOverLimitIsRefusedWhole(t *testing.T) {
	m := settingsModel(Options{
		ValidateToken: func(context.Context, string) error { return nil },
		SaveToken:     func(context.Context, string) error { return nil },
	})
	m = openSettingsForTest(t, m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	m = paste(m, strings.Repeat("a", maxTokenInputLength))
	if len([]rune(m.tokenInput)) != maxTokenInputLength {
		t.Fatalf("a full-length paste was not accepted: %d", len([]rune(m.tokenInput)))
	}
	m = paste(m, "b")
	if len([]rune(m.tokenInput)) != maxTokenInputLength {
		t.Fatalf("a paste past the limit was not refused: %d", len([]rune(m.tokenInput)))
	}
}

func TestBackgroundRefreshAndStartDoNotCloseOverlays(t *testing.T) {
	m := settingsModel(Options{
		Refresh: func(context.Context, RefreshRequest) RefreshResult { return RefreshResult{} },
	})
	m = openSettingsForTest(t, m)

	// A completed background refresh keeps the overlay open and updates data.
	refreshed := m.applyRefresh(RefreshResult{Generation: m.generation, Snapshot: ui.SyntheticLegacySnapshot()})
	if refreshed.state != ViewSettings {
		t.Fatalf("state = %v, want the overlay to stay open", refreshed.state)
	}
	if refreshed.snapshot.GeneratedAt.IsZero() {
		t.Fatal("the background refresh did not update the snapshot")
	}

	// An auth failure surfaces inside the overlay instead of forcing setup.
	failed := m.applyRefresh(RefreshResult{Generation: m.generation, AuthFailure: true, Err: errors.New("kimi: token is invalid or expired")})
	if failed.state != ViewSettings {
		t.Fatalf("state = %v, want the overlay to stay open on auth failure", failed.state)
	}
	if !failed.suspended || !failed.settingsErr {
		t.Fatalf("suspended=%v settingsErr=%v", failed.suspended, failed.settingsErr)
	}

	// Starting an attempt from inside the overlay must not change the state.
	started := m
	if _, ok := started.StartAttempt(false); !ok {
		t.Fatal("the background attempt did not start")
	}
	if started.state != ViewSettings {
		t.Fatalf("state = %v, want the overlay to survive a background attempt", started.state)
	}
}
