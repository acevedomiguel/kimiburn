package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultsAreApplied(t *testing.T) {
	cfg, err := Load(nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("refresh interval = %v, want 30s", cfg.RefreshInterval)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("base url = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.Theme != "auto" || cfg.Color != "auto" {
		t.Errorf("theme/color = %q/%q, want auto/auto", cfg.Theme, cfg.Color)
	}
	if !cfg.Keyring {
		t.Error("keyring must default to enabled")
	}
	if !cfg.AutoRefreshEnabled() {
		t.Error("the default interval must keep automatic refresh enabled")
	}
}

func TestValidateRejectsUnsupportedValues(t *testing.T) {
	valid := Config{
		Version:         1,
		RefreshInterval: 30 * time.Second,
		BaseURL:         DefaultBaseURL,
		Theme:           "auto",
		Color:           "auto",
		Keyring:         true,
	}

	tests := map[string]func(*Config){
		"wrong version":       func(c *Config) { c.Version = 2 },
		"refresh below floor": func(c *Config) { c.RefreshInterval = 29 * time.Second },
		"negative refresh":    func(c *Config) { c.RefreshInterval = -time.Second },
		"empty base":          func(c *Config) { c.BaseURL = "" },
		"http base":           func(c *Config) { c.BaseURL = "http://api.kimi.com/coding/v1" },
		"wrong path segment":  func(c *Config) { c.BaseURL = "https://api.kimi.com/coding/v2" },
		"arbitrary host":      func(c *Config) { c.BaseURL = "https://evil.example/coding/v1" },
		"open platform host":  func(c *Config) { c.BaseURL = "https://api.moonshot.ai/v1" },
		"openrouter host":     func(c *Config) { c.BaseURL = "https://openrouter.ai/api/v1" },
		"unknown theme":       func(c *Config) { c.Theme = "neon" },
		"unknown color":       func(c *Config) { c.Color = "sometimes" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected an error for %+v", cfg)
			}
		})
	}
}

func TestValidateAcceptsBothAllowedBases(t *testing.T) {
	for _, base := range []string{DefaultBaseURL, OverseasBaseURL} {
		cfg := Config{Version: 1, RefreshInterval: 30 * time.Second, BaseURL: base, Theme: "auto", Color: "auto"}
		if err := cfg.Validate(); err != nil {
			t.Errorf("base %q: unexpected error %v", base, err)
		}
	}
}

// TestAllowedBaseURLsAreExact matches the contract's "exactly these two"
// requirement: only the two literal bases pass, and every slash-suffixed or
// similarly-spelled variant is rejected rather than silently canonicalized.
func TestAllowedBaseURLsAreExact(t *testing.T) {
	accepted := map[string]bool{
		DefaultBaseURL:  true,
		OverseasBaseURL: true,

		DefaultBaseURL + "/":    false,
		DefaultBaseURL + "//":   false,
		DefaultBaseURL + "///":  false,
		OverseasBaseURL + "/":   false,
		OverseasBaseURL + "///": false,
		" " + DefaultBaseURL:    false,
		DefaultBaseURL + " ":    false,
		DefaultBaseURL + "/x":   false,
	}
	for base, want := range accepted {
		if got := IsAllowedBaseURL(base); got != want {
			t.Errorf("IsAllowedBaseURL(%q) = %v, want %v", base, got, want)
		}
		cfg := Config{Version: 1, RefreshInterval: 30 * time.Second, BaseURL: base, Theme: "auto", Color: "auto"}
		if err := cfg.Validate(); (err == nil) != want {
			t.Errorf("Validate(base %q) error = %v, want nil=%v", base, err, want)
		}
	}
}

func TestZeroRefreshIntervalDisablesAutoRefreshButStaysValid(t *testing.T) {
	cfg := Config{Version: 1, RefreshInterval: 0, BaseURL: DefaultBaseURL, Theme: "auto", Color: "auto"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.AutoRefreshEnabled() {
		t.Error("a zero interval must disable automatic refresh")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nsurprise: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{path}, nil)
	if err == nil {
		t.Fatal("expected a strict decode error for an unknown field")
	}
	if !strings.Contains(err.Error(), "surprise") {
		t.Errorf("error should name the offending field: %v", err)
	}
}

// TestTokenKeyIsAnUnknownField proves configuration can never carry a secret:
// a token-looking key is rejected by strict decoding.
func TestTokenKeyIsAnUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\ntoken: synthetic-not-a-real-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load([]string{path}, nil)
	if err == nil {
		t.Fatal("expected strict decoding to reject a token key")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error should name the offending key: %v", err)
	}
}

func TestLoadRejectsAdditionalYAMLDocuments(t *testing.T) {
	// Strict YAML must reject a second document instead of silently decoding
	// the first and ignoring the rest, which could hide unknown keys or an
	// accidental secret.
	cases := map[string]string{
		"unknown key in second doc": "version: 1\n---\nsurprise: true\n",
		"empty trailing doc":        "version: 1\n---\n",
		"second settings doc":       "version: 1\n---\nrefresh_interval: 90s\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load([]string{path}, nil); err == nil {
				t.Fatalf("expected an error for multiple YAML documents:\n%s", body)
			}
		})
	}
}

func TestMissingConfigFileIsNotAnError(t *testing.T) {
	cfg, err := Load([]string{filepath.Join(t.TempDir(), "absent.yaml")}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("refresh interval = %v, want defaults to survive a missing file", cfg.RefreshInterval)
	}
}

func TestPrecedenceYAMLThenEnvThenFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "version: 1\nrefresh_interval: 60s\nbase_url: https://api.kimi.ai/coding/v1\ntheme: dark\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// YAML alone beats the built-in defaults.
	cfg, err := Load([]string{path}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval != time.Minute {
		t.Errorf("refresh interval = %v, want 60s from YAML", cfg.RefreshInterval)
	}
	if cfg.BaseURL != OverseasBaseURL {
		t.Errorf("base url = %q, want the overseas YAML base", cfg.BaseURL)
	}

	// Environment beats the file.
	env := map[string]string{"KIMIBURN_REFRESH_INTERVAL": "45s", "KIMIBURN_BASE_URL": DefaultBaseURL, "KIMIBURN_THEME": "light"}
	cfg, err = Load([]string{path}, env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval != 45*time.Second {
		t.Errorf("refresh interval = %v, want 45s from env", cfg.RefreshInterval)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("base url = %q, want the default base from env", cfg.BaseURL)
	}
	if cfg.Theme != "light" {
		t.Errorf("theme = %q, want light from env", cfg.Theme)
	}

	// Flags beat the environment.
	flags := Overrides{RefreshInterval: durationPtr(90 * time.Second)}
	cfg, err = Load([]string{path}, env, flags)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval != 90*time.Second {
		t.Errorf("refresh interval = %v, want 90s from flags", cfg.RefreshInterval)
	}
}

func TestInvalidEnvValueIsRejected(t *testing.T) {
	tests := map[string]map[string]string{
		"unparsable interval": {"KIMIBURN_REFRESH_INTERVAL": "soon"},
		"negative interval":   {"KIMIBURN_REFRESH_INTERVAL": "-5s"},
		"below floor":         {"KIMIBURN_REFRESH_INTERVAL": "5s"},
		"unparsable keyring":  {"KIMIBURN_KEYRING": "perhaps"},
		"arbitrary base":      {"KIMIBURN_BASE_URL": "https://evil.example/coding/v1"},
	}

	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(nil, env); err == nil {
				t.Fatalf("expected an error for env %v", env)
			}
		})
	}
}

func TestFlagRefreshBelowFloorIsRejected(t *testing.T) {
	if _, err := Load(nil, nil, Overrides{RefreshInterval: durationPtr(2 * time.Second)}); err == nil {
		t.Fatal("expected an error for a 2s flag override")
	}
}

func TestFlagRefreshZeroIsAccepted(t *testing.T) {
	cfg, err := Load(nil, nil, Overrides{RefreshInterval: durationPtr(0)})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AutoRefreshEnabled() {
		t.Error("a zero flag override must disable automatic refresh")
	}
}

func TestNoColorOverrideDisablesColor(t *testing.T) {
	cfg, err := Load(nil, nil, Overrides{NoColor: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Color != "never" {
		t.Errorf("color = %q, want never after --no-color", cfg.Color)
	}
	if cfg.ColorsEnabled(true) {
		t.Error("ColorsEnabled must honor the --no-color flag")
	}

	// The default auto color setting enables color when the flag is absent.
	plain, err := Load(nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !plain.ColorsEnabled(false) {
		t.Error("the default auto color setting must enable color")
	}
}

func TestWriteSampleIsLoadableAndRestricted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")
	if err := WriteSample(path); err != nil {
		t.Fatalf("WriteSample: %v", err)
	}

	cfg, err := Load([]string{path}, nil)
	if err != nil {
		t.Fatalf("Load after WriteSample: %v", err)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("refresh interval = %v, want 30s", cfg.RefreshInterval)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "token") || strings.Contains(string(raw), "secret") {
		t.Errorf("sample config must never contain a credential:\n%s", raw)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if PermissionsAreEnforced {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("permissions = %o, want no group/world access", perm)
		}
	}
}

func TestLoadIgnoresEmptyEnvAndEmptyFlags(t *testing.T) {
	empty := ""
	cfg, err := Load(nil, map[string]string{"KIMIBURN_THEME": "", "KIMIBURN_BASE_URL": ""}, Overrides{BaseURL: &empty})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "auto" {
		t.Errorf("theme = %q, want the default when overrides are empty", cfg.Theme)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("base url = %q, want the default when overrides are empty", cfg.BaseURL)
	}
}

func TestUnreadableConfigPathIsReported(t *testing.T) {
	// A directory can never be decoded as configuration; that must surface as
	// an error rather than being mistaken for a missing file.
	dir := t.TempDir()
	if _, err := Load([]string{dir}, nil); err == nil {
		t.Fatal("expected an error when the config path is not a regular file")
	}
}

func TestSlashSuffixedBaseURLIsRejected(t *testing.T) {
	if _, err := Load(nil, map[string]string{"KIMIBURN_BASE_URL": OverseasBaseURL + "///"}); err == nil {
		t.Fatal("expected a trailing-slash base to be rejected, not canonicalized")
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }
