package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
)

// defaultDeps returns the production seams. Token validation stays nil until
// S1.4 supplies the bounded Kimi usage client; setup must never fabricate a
// successful validation.
func defaultDeps() deps {
	return deps{
		validate:    nil,
		keyring:     credentials.NewOSKeyring(),
		interactive: isTerminal,
		readSecret:  readSecretNoEcho,
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

// storeFor builds the credential store for the resolved paths. Only Kimiburn's
// own keyring service and fallback file are ever referenced.
func storeFor(paths config.Paths, opts options, d deps) (*credentials.Store, error) {
	cfg, err := resolveConfig(paths, opts)
	if err != nil {
		return nil, err
	}
	kr := d.keyring
	if !cfg.Keyring {
		kr = nil
	}
	return credentials.NewStore(credentials.Options{
		Keyring:        kr,
		TokenFile:      paths.TokenFile(),
		KeyringEnabled: cfg.Keyring,
	}), nil
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
func doLogout(paths config.Paths, opts options, stdout, stderr io.Writer, d deps) int {
	store, err := storeFor(paths, opts, d)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: logout: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	if err := store.Logout(context.Background()); err != nil {
		fmt.Fprintf(stderr, "kimiburn: logout: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	fmt.Fprintln(stdout, "Local Kimiburn credentials removed.")
	fmt.Fprintln(stdout, "The token remains valid at Kimi. Local configuration and cached usage data were not deleted.")
	return exitOK
}
