package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acevedomiguel/burnblocks/kimi"
	"github.com/acevedomiguel/kimiburn/internal/app"
)

// errorReader fails on any read, so a CLI path that prompts or consumes stdin
// fails its test instead of hanging on a terminal.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("stdin must not be read on this path")
}

// isolatedEnv points every config/cache location at a fresh directory so the
// help and version paths are proven to run without user state.
func isolatedEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("KIMIBURN_TOKEN", "")
	return home
}

// runCLI executes the CLI with buffered streams and an unusable stdin.
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, errorReader{}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func assertNoStateWritten(t *testing.T, home string) {
	t.Helper()
	err := filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != home {
			t.Errorf("help/version wrote %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", home, err)
	}
}

func TestIsAuthFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "wrapped unauthorized", err: errors.Join(errors.New("synthetic wrapper"), kimi.ErrUnauthorized), want: true},
		{name: "wrapped forbidden", err: errors.Join(errors.New("synthetic wrapper"), kimi.ErrForbidden), want: true},
		{name: "transport", err: kimi.ErrTransport, want: false},
		{name: "nil", err: nil, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isAuthFailure(test.err); got != test.want {
				t.Fatalf("isAuthFailure() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRunHelpPrintsUsageOnStdout(t *testing.T) {
	home := isolatedEnv(t)
	for _, args := range [][]string{{"--help"}, {"-help"}, {"-h"}} {
		code, stdout, stderr := runCLI(args...)
		if code != exitOK {
			t.Errorf("%v: exit code = %d, want %d", args, code, exitOK)
		}
		for _, want := range []string{"Usage:", "--version", "Exit codes:"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: stdout missing %q in:\n%s", args, want, stdout)
			}
		}
		if stderr != "" {
			t.Errorf("%v: stderr = %q, want empty", args, stderr)
		}
	}
	assertNoStateWritten(t, home)
}

func TestRunVersionPrintsVersionOnStdout(t *testing.T) {
	home := isolatedEnv(t)
	want := "kimiburn version " + version + "\n"
	for _, args := range [][]string{{"--version"}, {"-version"}} {
		code, stdout, stderr := runCLI(args...)
		if code != exitOK {
			t.Errorf("%v: exit code = %d, want %d", args, code, exitOK)
		}
		if stdout != want {
			t.Errorf("%v: stdout = %q, want %q", args, stdout, want)
		}
		if stderr != "" {
			t.Errorf("%v: stderr = %q, want empty", args, stderr)
		}
	}
	assertNoStateWritten(t, home)
}

func TestRunWithoutTokenRunsSetupThenDashboard(t *testing.T) {
	isolatedEnv(t)
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.keyring = nil
	setupCalls := 0
	d.setup = func(kimi.Paths, options, io.Reader, io.Writer, io.Writer, deps) int {
		setupCalls++
		t.Setenv(kimi.TokenEnvVar, "test-token")
		return exitOK
	}
	d.runDashboard = func(model app.Model, _ io.Reader, _ io.Writer) error {
		updated, cmd := model.Update(model.Init()())
		if cmd == nil {
			t.Fatal("live dashboard did not start a refresh")
		}
		got := updated.(app.Model).View().Content
		if strings.Contains(got, "Token setup required") || !strings.Contains(got, "Loading latest Kimi Code quota") {
			t.Fatalf("dashboard did not follow successful setup: %s", got)
		}
		return nil
	}
	if code := runWithDeps(nil, errorReader{}, &out, &errOut, d); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if setupCalls != 1 {
		t.Fatalf("setup calls = %d, want 1", setupCalls)
	}
}

func TestRunDoesNotStartDashboardWhenAutomaticSetupFails(t *testing.T) {
	isolatedEnv(t)
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.keyring = nil
	d.setup = func(kimi.Paths, options, io.Reader, io.Writer, io.Writer, deps) int { return exitFailure }
	d.runDashboard = func(app.Model, io.Reader, io.Writer) error {
		t.Fatal("dashboard started after setup failed")
		return nil
	}
	if code := runWithDeps(nil, errorReader{}, &out, &errOut, d); code != exitFailure {
		t.Fatalf("exit=%d, want %d", code, exitFailure)
	}
}

func TestRunWithTokenSkipsAutomaticSetup(t *testing.T) {
	isolatedEnv(t)
	t.Setenv(kimi.TokenEnvVar, "already-configured")
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.setup = func(kimi.Paths, options, io.Reader, io.Writer, io.Writer, deps) int {
		t.Fatal("setup ran despite an existing token")
		return exitFailure
	}
	d.runDashboard = func(app.Model, io.Reader, io.Writer) error { return nil }
	if code := runWithDeps(nil, errorReader{}, &out, &errOut, d); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
}

func TestDemoRunsSyntheticDashboardWithoutProvider(t *testing.T) {
	isolatedEnv(t)
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.newUsage = func(string, string) (kimi.UsageSource, error) {
		t.Fatal("demo constructed provider")
		return nil, nil
	}
	d.runDashboard = func(model app.Model, _ io.Reader, _ io.Writer) error {
		updated, cmd := model.Update(model.Init()())
		if cmd == nil {
			t.Fatal("demo did not produce synthetic snapshot")
		}
		updated, _ = updated.Update(cmd())
		if got := updated.(app.Model).View().Content; !strings.Contains(got, "Monthly membership") || !strings.Contains(got, "SYNTHETIC") {
			t.Fatalf("demo view: %s", got)
		}
		return nil
	}
	for _, args := range [][]string{{"demo"}, {"demo", "--no-color"}} {
		if code := runWithDeps(args, errorReader{}, &out, &errOut, d); code != exitOK {
			t.Fatalf("args=%v exit=%d stderr=%s", args, code, errOut.String())
		}
	}
}

func TestRunUnknownFlagIsUsageError(t *testing.T) {
	isolatedEnv(t)
	code, stdout, stderr := runCLI("--frobnicate")
	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "kimiburn: ") {
		t.Errorf("stderr missing diagnosis prefix in:\n%s", stderr)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr missing usage reminder in:\n%s", stderr)
	}
}

// The help output names the shared engine the binary embeds, so a support
// report can identify it after the engine moved to its own module.
func TestHelpReportsEngineVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout.String(), "github.com/acevedomiguel/burnblocks") {
		t.Errorf("help must name the engine module:\n%s", stdout.String())
	}
	if got := engineVersion(); got == "" {
		t.Error("engineVersion must never be empty")
	}
}
