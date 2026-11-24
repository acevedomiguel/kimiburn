package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configEnvPointsAt redirects config/cache resolution into a temp dir and
// returns the resolved config file so the assertions are platform independent.
func configEnvPointsAt(t *testing.T) (configDir, cacheDir string) {
	t.Helper()
	home := t.TempDir()
	configDir = filepath.Join(home, "config")
	cacheDir = filepath.Join(home, "cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("KIMIBURN_TOKEN", "")
	return configDir, cacheDir
}

func TestConfigPathPrintsConfigFile(t *testing.T) {
	configDir, cacheDir := configEnvPointsAt(t)
	code, stdout, stderr := runCLI("config", "path")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	want := filepath.Join(configDir, "kimiburn", "config.yaml") + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	// `config path` is read-only: it must not create any file.
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("cache dir should not exist after `config path`: %v", err)
	}
}

func TestConfigPathHonorsExplicitConfigFlag(t *testing.T) {
	configEnvPointsAt(t)
	explicit := filepath.Join(t.TempDir(), "custom.yaml")
	code, stdout, stderr := runCLI("--config", explicit, "config", "path")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if stdout != explicit+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, explicit)
	}
}

func TestConfigResetWritesSampleConfigOnly(t *testing.T) {
	configDir, cacheDir := configEnvPointsAt(t)
	code, stdout, stderr := runCLI("config", "reset")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	want := filepath.Join(configDir, "kimiburn", "config.yaml")
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to name %q", stdout, want)
	}

	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("reading sample config: %v", err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "token") {
		t.Errorf("reset config must not contain a token:\n%s", raw)
	}

	// Reset must never touch the cache tree.
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("cache dir should not exist after `config reset`: %v", err)
	}
}

func TestConfigResetRefusesToWriteOutsideKimiburnPathsWithoutConfigFlag(t *testing.T) {
	// A bare `config reset` writes only the default Kimiburn config file. It
	// must never fall back to deleting or overwriting arbitrary user files.
	configDir, _ := configEnvPointsAt(t)
	canary := filepath.Join(configDir, "unrelated.txt")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canary, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runCLI("config", "reset")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if raw, err := os.ReadFile(canary); err != nil || string(raw) != "keep me" {
		t.Errorf("config reset clobbered an unrelated file: %q err=%v", raw, err)
	}
}

func TestConfigSubcommandWithoutActionIsUsageError(t *testing.T) {
	configEnvPointsAt(t)
	code, stdout, stderr := runCLI("config")
	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "config") {
		t.Errorf("stderr should explain the config usage: %q", stderr)
	}
}
