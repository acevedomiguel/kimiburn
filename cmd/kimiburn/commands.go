package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
	"github.com/acevedomiguel/kimiburn/internal/domain"
	"github.com/acevedomiguel/kimiburn/internal/kimi"
)

// defaultDeps returns the production seams. Token validation is the bounded
// Kimi usage client; setup must never fabricate a successful validation.
func defaultDeps() deps {
	return deps{
		validateFactory: validateToken,
		keyring:         credentials.NewOSKeyring(),
		interactive:     isTerminal,
		readSecret:      readSecretNoEcho,
		newUsage: func(baseURL, token string) (domain.UsageSource, error) {
			return kimi.NewClient(kimi.WithBaseURL(baseURL), kimi.WithToken(token), kimi.WithVersion(version))
		},
		now: time.Now,
	}
}

// configPathsFor picks the configuration file, honoring an explicit flag.
func configPathsFor(paths config.Paths, opts options) []string {
	if opts.configPath != "" {
		return []string{opts.configPath}
	}
	return []string{paths.ConfigFile}
}

// environment collects the variables the configuration layer understands.
func environment() map[string]string {
	env := map[string]string{}
	for _, key := range []string{
		"KIMIBURN_REFRESH_INTERVAL",
		"KIMIBURN_BASE_URL",
		"KIMIBURN_THEME",
		"KIMIBURN_COLOR",
		"KIMIBURN_KEYRING",
	} {
		if v, ok := os.LookupEnv(key); ok {
			env[key] = v
		}
	}
	return env
}

// resolveConfig loads strict configuration so setup and logout honour the
// documented `keyring` setting rather than guessing from the environment.
func resolveConfig(paths config.Paths, opts options) (config.Config, error) {
	return config.Load(configPathsFor(paths, opts), environment())
}

// earlyRedact scrubs a diagnostic produced before the credential store exists.
// The environment token is the only configured secret already known there, so
// it is added to the generic shapes; this keeps an invalid configuration value
// that happens to equal the token from echoing it back.
func earlyRedact(s string) string {
	return credentials.NewRedactor(os.Getenv(credentials.EnvToken)).Redact(s)
}

// storeFor builds the credential store for setup, honoring the configured
// keyring preference. Only Kimiburn's own keyring service and fallback file are
// ever referenced.
func storeFor(paths config.Paths, opts options, d deps) (*credentials.Store, error) {
	cfg, err := resolveConfig(paths, opts)
	if err != nil {
		return nil, err
	}
	// The keyring is passed through even when configuration disables it, so a
	// disabled keyring can still clear a stale entry from an earlier run. A
	// disabled keyring never gates the file save.
	return credentials.NewStore(credentials.Options{
		Keyring:        d.keyring,
		TokenFile:      paths.TokenFile(),
		KeyringEnabled: cfg.Keyring,
	}), nil
}

// logoutStoreFor builds the store used by logout. Logout must not depend on
// configuration being valid or on the `keyring` preference: it always attempts
// Kimiburn's own keyring entry and fallback file so flipping that setting cannot
// strand a previously stored token.
func logoutStoreFor(paths config.Paths, d deps) *credentials.Store {
	return credentials.NewStore(credentials.Options{
		Keyring:        d.keyring,
		TokenFile:      paths.TokenFile(),
		KeyringEnabled: d.keyring != nil,
	})
}

// writeSampleIfAbsent writes the sample configuration only when no file exists,
// so a hand-edited configuration keeps its backend, theme, and interval. It
// reports whether the sample was written.
func writeSampleIfAbsent(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("config: inspect %s: %w", path, err)
	}
	if err := config.WriteSample(path); err != nil {
		return false, err
	}
	return true, nil
}

// doConfigReset restores the sample configuration file.
func doConfigReset(paths config.Paths, opts options, stdout, stderr io.Writer) int {
	path := paths.ConfigFile
	if opts.configPath != "" {
		path = opts.configPath
	}
	if err := config.WriteSample(path); err != nil {
		fmt.Fprintf(stderr, "kimiburn: config reset: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)
	return exitOK
}

// doLogout removes locally stored Kimiburn credentials. It never revokes the
// token at Kimi, because Kimiburn has no remote write access.
func doLogout(paths config.Paths, stdout, stderr io.Writer, d deps) int {
	store := logoutStoreFor(paths, d)
	if err := store.Logout(context.Background()); err != nil {
		fmt.Fprintf(stderr, "kimiburn: logout: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	fmt.Fprintln(stdout, "Local Kimiburn credentials removed.")
	fmt.Fprintln(stdout, "The token remains valid at Kimi. Local configuration and cached usage data were not deleted.")
	return exitOK
}
