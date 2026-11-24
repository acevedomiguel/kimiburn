package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Paths holds the resolved locations Kimiburn reads and writes.
type Paths struct {
	ConfigDir  string
	ConfigFile string
	CacheDir   string
	CacheFile  string
}

// TokenFile is the permission-restricted fallback for the Kimi Code token. It
// is used only when the environment and OS keyring do not supply one.
func (p Paths) TokenFile() string {
	return filepath.Join(p.ConfigDir, "token")
}

// DefaultPaths resolves platform-appropriate directories through the standard
// library so XDG and OS overrides are honoured automatically. Config lives
// under the user config directory and cache under the user cache directory, so
// `config path` and `config reset` never reach into the cache tree.
func DefaultPaths() (Paths, error) {
	configDir, err := userDir("XDG_CONFIG_HOME", os.UserConfigDir)
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolve user config dir: %w", err)
	}
	cacheDir, err := userDir("XDG_CACHE_HOME", os.UserCacheDir)
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolve user cache dir: %w", err)
	}

	p := Paths{
		ConfigDir: filepath.Join(configDir, "kimiburn"),
		CacheDir:  filepath.Join(cacheDir, "kimiburn"),
	}
	p.ConfigFile = filepath.Join(p.ConfigDir, "config.yaml")
	p.CacheFile = filepath.Join(p.CacheDir, "state.json")
	return p, nil
}

func parentDir(path string) string {
	dir := filepath.Dir(path)
	if dir == "" {
		return "."
	}
	return dir
}

// userDir prefers an explicit absolute XDG directory when the user set one,
// and falls back to the platform-native location otherwise. XDG requires
// absolute paths, so a relative override is invalid: it is ignored rather than
// making the resolved paths depend on the process working directory. The
// standard library fallback itself rejects a relative XDG variable (on Linux
// os.UserConfigDir errors), so the invalid value is temporarily suppressed for
// the duration of the fallback and restored afterwards. On macOS the standard
// library ignores XDG, but users who set it expect it to win.
func userDir(envVar string, fallback func() (string, error)) (string, error) {
	v := strings.TrimSpace(os.Getenv(envVar))
	if v != "" && filepath.IsAbs(v) {
		return v, nil
	}

	// A set-but-relative variable must not leak into the fallback, because the
	// fallback may reject it. Suppress it, run the fallback, then restore the
	// original value on every path.
	set := false
	if _, ok := os.LookupEnv(envVar); ok {
		set = true
		if err := os.Unsetenv(envVar); err != nil {
			return "", err
		}
	}
	defer func() {
		if set {
			_ = os.Setenv(envVar, v)
		}
	}()
	return fallback()
}
