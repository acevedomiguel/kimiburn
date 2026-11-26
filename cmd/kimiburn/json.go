package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

// doJSON performs one live read and emits exactly one schema-1 document. It
// never prompts: credential discovery is limited to the existing store.
func doJSON(paths kimi.Paths, opts options, stdout, stderr io.Writer, d deps) int {
	overrides := kimi.Overrides{NoColor: opts.noColor}
	if opts.refreshSet {
		overrides.RefreshInterval = &opts.refreshInterval
	}
	cfg, err := kimi.LoadConfig(configPathsFor(paths, opts), environment(), overrides)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return exitUsage
	}

	credentialStore := kimi.NewTokenStore(kimi.TokenStoreOptions{
		Keyring: d.keyring, TokenFile: paths.TokenFile(), KeyringEnabled: cfg.Keyring,
	})
	token, err := credentialStore.Resolve(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: credentials: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	if token == "" {
		fmt.Fprintln(stderr, "kimiburn: no Kimi Code token is configured. Run `kimiburn setup` or set KIMIBURN_TOKEN.")
		return exitCredentials
	}
	redactor := kimi.NewRedactor(token, credentialStore.EnvironmentToken())

	stateStore := d.stateStore
	if stateStore == nil {
		if opts.noCache {
			stateStore = kimi.NewDisabledStateStore()
		} else {
			stateStore = kimi.NewStateStore(paths.CacheFile)
		}
	}
	state := kimi.EmptyStoredState()
	var cacheWarning *kimi.Warning
	if !opts.noCache {
		state, err = stateStore.Load()
		if err != nil {
			state = kimi.EmptyStoredState()
			warning := kimi.Warning{Section: "Cache", Message: "ignored cache: " + redactor.Redact(err.Error())}
			cacheWarning = &warning
		}
	}

	if d.newUsage == nil {
		fmt.Fprintln(stderr, "kimiburn: provider reader is unavailable")
		return exitFailure
	}
	source, err := d.newUsage(cfg.BaseURL, token)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", redactor.Redact(err.Error()))
		return exitFailure
	}
	reading, fetchErr := source.ReadUsage(context.Background())
	if fetchErr == nil {
		snapshot := kimi.BuildSnapshot(reading, &state)
		if cacheWarning != nil {
			snapshot.Warnings = append(snapshot.Warnings, *cacheWarning)
			// Keep the persisted snapshot identical to the printed one.
			if state.Snapshot != nil {
				state.Snapshot.Warnings = append(state.Snapshot.Warnings, *cacheWarning)
			}
		}
		if !opts.noCache {
			if err := stateStore.Save(state); err != nil {
				snapshot.Warnings = append(snapshot.Warnings, kimi.Warning{Section: "Cache", Message: "could not save state: " + redactor.Redact(err.Error())})
			}
		}
		return writeJSONProjection(stdout, stderr, snapshot, cfg.BaseURL, false, d.now, redactor)
	}

	if !opts.noCache && state.Snapshot != nil {
		snapshot := *state.Snapshot
		snapshot.Warnings = append([]kimi.Warning{}, state.Snapshot.Warnings...)
		snapshot.Warnings = append(snapshot.Warnings, kimi.Warning{Section: "Refresh", Message: redactor.Redact(fetchErr.Error())})
		return writeJSONProjection(stdout, stderr, snapshot, cfg.BaseURL, true, d.now, redactor)
	}
	if cacheWarning != nil {
		fmt.Fprintf(stderr, "kimiburn: cache: %s\n", cacheWarning.Message)
	}
	fmt.Fprintf(stderr, "kimiburn: %s\n", redactor.Redact(fetchErr.Error()))
	return exitFailure
}

func writeJSONProjection(stdout, stderr io.Writer, snapshot kimi.Snapshot, baseURL string, cached bool, now func() time.Time, redactor *kimi.Redactor) int {
	if now == nil {
		now = time.Now
	}
	if err := kimi.WriteJSON(stdout, snapshot, kimi.JSONOptions{APIBase: baseURL, Cached: cached, Now: now()}); err != nil {
		message := err.Error()
		if redactor != nil {
			message = redactor.Redact(message)
		} else {
			message = kimi.Redact(message)
		}
		fmt.Fprintf(stderr, "kimiburn: %s\n", message)
		return exitFailure
	}
	return exitOK
}
