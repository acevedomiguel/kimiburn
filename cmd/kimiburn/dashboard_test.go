package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/acevedomiguel/kimiburn/internal/app"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// readyDashboard runs the CLI with a dashboard stub, then drives the captured
// model through its first refresh so it reaches the ready state. It returns the
// model and the kimiburn configuration directory.
func readyDashboard(t *testing.T, args []string, d deps) (app.Model, string) {
	t.Helper()
	kimiburnDir := setupEnv(t)
	var captured app.Model
	d.runDashboard = func(model app.Model, _ io.Reader, _ io.Writer) error {
		captured = model
		return nil
	}
	code, _, stderr := runWith(args, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}

	updated, cmd := captured.Update(captured.Init()())
	m := updated.(app.Model)
	if cmd == nil {
		t.Fatal("the dashboard did not start its first refresh")
	}
	updated, _ = m.Update(cmd())
	return updated.(app.Model), kimiburnDir
}

// settingsDeps wires a fake keyring, cache, and provider so no test touches the
// network or real user state.
func settingsDeps(kr credentials.Keyring, source domain.UsageSource, now time.Time, validate func(context.Context, string) error) deps {
	d := defaultDeps()
	d.keyring = kr
	d.stateStore = &memoryStateStore{state: domain.EmptyStoredState()}
	d.newUsage = func(string, string) (domain.UsageSource, error) { return source, nil }
	d.validate = validate
	d.now = func() time.Time { return now }
	return d
}

func TestDashboardSettingsPersistIntervalAndStoreToken(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	kr := newCLIKeyring()
	if err := kr.Set(credentials.ServiceName, credentials.AccountName, "existing-token"); err != nil {
		t.Fatal(err)
	}
	var validated string
	d := settingsDeps(kr, &fakeUsageSource{reading: syntheticReading(now)}, now, func(_ context.Context, token string) error {
		validated = token
		return nil
	})
	m, kimiburnDir := readyDashboard(t, nil, d)

	// Open Settings and choose 1m.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(app.Model)
	if view := m.View().Content; !strings.Contains(view, "Refresh interval") || !strings.Contains(view, "Kimi Code token") {
		t.Fatalf("settings overlay missing its rows:\n%s", view)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(app.Model)

	cfgPath := filepath.Join(kimiburnDir, "config.yaml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("the interval change did not persist to %s: %v", cfgPath, err)
	}
	if !strings.Contains(string(raw), "refresh_interval: 1m") {
		t.Errorf("saved config missing the new interval:\n%s", raw)
	}
	if strings.Contains(strings.ToLower(string(raw)), "token") {
		t.Errorf("the saved config must never contain a token:\n%s", raw)
	}

	// Replace the token through the overlay.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(app.Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(app.Model)
	const replacement = "km_synthetic_replacement_value"
	for _, r := range replacement {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(app.Model)
	}
	if view := m.View().Content; strings.Contains(view, replacement) {
		t.Fatalf("the typed token appeared in the overlay:\n%s", view)
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(app.Model)
	if cmd == nil {
		t.Fatal("submitting the token returned no command")
	}
	updated, _ = m.Update(cmd())
	_ = updated.(app.Model)

	if validated != replacement {
		t.Errorf("validated = %q, want the replacement", validated)
	}
	if got := kr.values[credentials.ServiceName+"/"+credentials.AccountName]; got != replacement {
		t.Errorf("keyring = %q, want the replacement token", got)
	}
}

func TestDashboardSettingsFlagsExternalIntervalOverride(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	kr := newCLIKeyring()
	if err := kr.Set(credentials.ServiceName, credentials.AccountName, "existing-token"); err != nil {
		t.Fatal(err)
	}
	d := settingsDeps(kr, &fakeUsageSource{reading: syntheticReading(now)}, now, func(context.Context, string) error { return nil })
	m, _ := readyDashboard(t, []string{"--refresh", "30s"}, d)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(app.Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(app.Model)
	if view := m.View().Content; !strings.Contains(view, "KIMIBURN_REFRESH_INTERVAL") {
		t.Fatalf("the overlay should warn that an external override wins:\n%s", view)
	}
}

func TestDashboardSettingsEnvironmentTokenIsExplained(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.UTC)
	setupEnv(t)
	t.Setenv(credentials.EnvToken, "env-synthetic-token")
	d := defaultDeps()
	d.stateStore = &memoryStateStore{state: domain.EmptyStoredState()}
	d.newUsage = func(string, string) (domain.UsageSource, error) {
		return &fakeUsageSource{reading: syntheticReading(now)}, nil
	}
	d.now = func() time.Time { return now }

	var captured app.Model
	d.runDashboard = func(model app.Model, _ io.Reader, _ io.Writer) error {
		captured = model
		return nil
	}
	if code, _, stderr := runWith(nil, errorReader{}, d); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	updated, cmd := captured.Update(captured.Init()())
	m := updated.(app.Model)
	updated, _ = m.Update(cmd())
	m = updated.(app.Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(app.Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(app.Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(app.Model)
	if view := m.View().Content; !strings.Contains(view, "KIMIBURN_TOKEN") {
		t.Fatalf("the overlay should explain the environment token:\n%s", view)
	}
}
