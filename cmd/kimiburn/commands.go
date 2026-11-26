package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

// defaultDeps returns the production seams. Token validation is the bounded
// Kimi usage client; setup must never fabricate a successful validation.
func defaultDeps() deps {
	return deps{
		validateFactory: validateToken,
		keyring:         kimi.NewOSKeyring(),
		interactive:     isTerminal,
		readSecret:      readSecretNoEcho,
		newUsage: func(baseURL, token string) (kimi.UsageSource, error) {
			return kimi.NewSource(baseURL, token, kimi.WithVersion(version))
		},
		now:          time.Now,
		runDashboard: runTerminal,
		setup:        doSetup,
	}
}

// configPathsFor picks the configuration file, honoring an explicit flag.
func isAuthFailure(err error) bool {
	return errors.Is(err, kimi.ErrUnauthorized) || errors.Is(err, kimi.ErrForbidden)
}

func configPathsFor(paths kimi.Paths, opts options) []string {
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
func resolveConfig(paths kimi.Paths, opts options) (kimi.Config, error) {
	return kimi.LoadConfig(configPathsFor(paths, opts), environment())
}

// earlyRedact scrubs a diagnostic produced before the credential store exists.
// The environment token is the only configured secret already known there, so
// it is added to the generic shapes; this keeps an invalid configuration value
// that happens to equal the token from echoing it back.
func earlyRedact(s string) string {
	return kimi.NewRedactor(os.Getenv(kimi.TokenEnvVar)).Redact(s)
}

// storeFor builds the credential store for setup, honoring the configured
// keyring preference. Only Kimiburn's own keyring service and fallback file are
// ever referenced.
func storeFor(paths kimi.Paths, opts options, d deps) (*kimi.TokenStore, error) {
	cfg, err := resolveConfig(paths, opts)
	if err != nil {
		return nil, err
	}
	// The keyring is passed through even when configuration disables it, so a
	// disabled keyring can still clear a stale entry from an earlier run. A
	// disabled keyring never gates the file save.
	return kimi.NewTokenStore(kimi.TokenStoreOptions{
		Keyring:        d.keyring,
		TokenFile:      paths.TokenFile(),
		KeyringEnabled: cfg.Keyring,
	}), nil
}

// logoutStoreFor builds the store used by logout. Logout must not depend on
// configuration being valid or on the `keyring` preference: it always attempts
// Kimiburn's own keyring entry and fallback file so flipping that setting cannot
// strand a previously stored token.
func logoutStoreFor(paths kimi.Paths, d deps) *kimi.TokenStore {
	return kimi.NewTokenStore(kimi.TokenStoreOptions{
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
	if err := kimi.WriteSample(path); err != nil {
		return false, err
	}
	return true, nil
}

// doConfigReset restores the sample configuration file.
func doConfigReset(paths kimi.Paths, opts options, stdout, stderr io.Writer) int {
	path := paths.ConfigFile
	if opts.configPath != "" {
		path = opts.configPath
	}
	if err := kimi.WriteSample(path); err != nil {
		fmt.Fprintf(stderr, "kimiburn: config reset: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)
	return exitOK
}

// doLogout removes locally stored Kimiburn credentials. It never revokes the
// token at Kimi, because Kimiburn has no remote write access.
func doLogout(paths kimi.Paths, stdout, stderr io.Writer, d deps) int {
	store := logoutStoreFor(paths, d)
	if err := store.Logout(context.Background()); err != nil {
		fmt.Fprintf(stderr, "kimiburn: logout: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	fmt.Fprintln(stdout, "Local Kimiburn credentials removed.")
	fmt.Fprintln(stdout, "The token remains valid at Kimi. Local configuration and cached usage data were not deleted.")
	return exitOK
}
