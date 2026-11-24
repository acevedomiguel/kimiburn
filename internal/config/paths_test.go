package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathsUseKimiburnUserDirs(t *testing.T) {
	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if !strings.HasSuffix(p.ConfigFile, filepath.Join("kimiburn", "config.yaml")) {
		t.Errorf("config file = %q", p.ConfigFile)
	}
	if !strings.HasSuffix(p.CacheFile, filepath.Join("kimiburn", "state.json")) {
		t.Errorf("cache file = %q", p.CacheFile)
	}
	if got, want := p.TokenFile(), filepath.Join(p.ConfigDir, "token"); got != want {
		t.Errorf("token file = %q, want %q", got, want)
	}
}

func TestPathsHonorXDGEnvironment(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg-config")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if want := filepath.Join(configHome, "kimiburn", "config.yaml"); p.ConfigFile != want {
		t.Errorf("config file = %q, want %q", p.ConfigFile, want)
	}
	if want := filepath.Join(cacheHome, "kimiburn", "state.json"); p.CacheFile != want {
		t.Errorf("cache file = %q, want %q", p.CacheFile, want)
	}
}

func TestPathsIsolateConfigFromCache(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg-config")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	// The config and cache trees must be separate so `config path`/`reset`
	// never confuse the two, and `config reset` never touches the cache.
	if strings.HasPrefix(p.CacheFile, p.ConfigDir) {
		t.Errorf("cache file %q must not live under the config dir %q", p.CacheFile, p.ConfigDir)
	}
	if strings.HasPrefix(p.ConfigFile, p.CacheDir) {
		t.Errorf("config file %q must not live under the cache dir %q", p.ConfigFile, p.CacheDir)
	}
}

func TestPathsIgnoreRelativeXDGOverrides(t *testing.T) {
	// XDG requires absolute paths; a relative override is invalid and must be
	// ignored rather than making the resolved paths depend on the process cwd.
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("XDG_CACHE_HOME", "relative-cache")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if !filepath.IsAbs(p.ConfigFile) || !filepath.IsAbs(p.CacheFile) {
		t.Errorf("relative XDG overrides produced non-absolute paths: config=%q cache=%q", p.ConfigFile, p.CacheFile)
	}
	if strings.HasPrefix(p.ConfigFile, "relative-config") || strings.HasPrefix(p.CacheFile, "relative-cache") {
		t.Errorf("relative XDG overrides were not ignored: config=%q cache=%q", p.ConfigFile, p.CacheFile)
	}

	base, err := os.UserConfigDir()
	if err == nil && !strings.HasPrefix(p.ConfigFile, base) {
		t.Errorf("config file = %q, want the platform fallback under %q", p.ConfigFile, base)
	}
}

func TestUserDirSuppressesRelativeOverrideDuringFallback(t *testing.T) {
	// Platform-independent guard for the Linux failure mode: when the relative
	// XDG variable is ignored, the fallback must not see it, and the original
	// value must be restored afterwards regardless of the fallback outcome.
	t.Setenv("XDG_CONFIG_HOME", "relative-config")

	sawValue := "sentinel"
	_, err := userDir("XDG_CONFIG_HOME", func() (string, error) {
		if v, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok {
			sawValue = v
		}
		return "/fallback/config", nil
	})
	if err != nil {
		t.Fatalf("userDir: %v", err)
	}
	if sawValue != "sentinel" {
		t.Errorf("fallback saw XDG_CONFIG_HOME=%q, want it absent", sawValue)
	}
	if got := os.Getenv("XDG_CONFIG_HOME"); got != "relative-config" {
		t.Errorf("XDG_CONFIG_HOME = %q after fallback, want the original restored", got)
	}

	// The value is restored even when the fallback itself fails.
	fallbackErr := errors.New("fallback failed")
	if _, err := userDir("XDG_CONFIG_HOME", func() (string, error) { return "", fallbackErr }); !errors.Is(err, fallbackErr) {
		t.Fatalf("userDir error = %v, want %v", err, fallbackErr)
	}
	if got := os.Getenv("XDG_CONFIG_HOME"); got != "relative-config" {
		t.Errorf("XDG_CONFIG_HOME = %q after failed fallback, want the original restored", got)
	}
}

func TestPathsFallBackToOSDefaults(t *testing.T) {
	// With XDG unset the platform-native user dirs are used; the leaf layout is
	// unchanged.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir on this platform: %v", err)
	}
	if !strings.HasPrefix(p.ConfigFile, base) {
		t.Errorf("config file = %q, want it under %q", p.ConfigFile, base)
	}
}
