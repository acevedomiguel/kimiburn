package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acevedomiguel/kimiburn/internal/app"
	"github.com/acevedomiguel/kimiburn/internal/domain"
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

func TestRunWithoutArgsInvokesDashboardEntryPoint(t *testing.T) {
	isolatedEnv(t)
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.keyring = nil
	d.runDashboard = func(model app.Model, _ io.Reader, _ io.Writer) error {
		updated, cmd := model.Update(model.Init()())
		if cmd != nil {
			updated, _ = updated.Update(cmd())
		}
		if got := updated.(app.Model).View().Content; !strings.Contains(got, "Token setup required") {
			t.Fatalf("root view: %s", got)
		}
		return nil
	}
	if code := runWithDeps(nil, errorReader{}, &out, &errOut, d); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
}

func TestDemoRunsSyntheticDashboardWithoutProvider(t *testing.T) {
	isolatedEnv(t)
	var out, errOut bytes.Buffer
	d := defaultDeps()
	d.newUsage = func(string, string) (domain.UsageSource, error) {
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
	if code := runWithDeps([]string{"demo"}, errorReader{}, &out, &errOut, d); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
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
