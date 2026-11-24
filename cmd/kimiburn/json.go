package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/app"
	"github.com/acevedomiguel/kimiburn/internal/cache"
	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
	"github.com/acevedomiguel/kimiburn/internal/domain"
	"github.com/acevedomiguel/kimiburn/internal/runway"
)

// doJSON performs one live read and emits exactly one schema-1 document. It
// never prompts: credential discovery is limited to the existing store.
func doJSON(paths config.Paths, opts options, stdout, stderr io.Writer, d deps) int {
	overrides := config.Overrides{NoColor: opts.noColor}
	if opts.refreshSet {
		overrides.RefreshInterval = &opts.refreshInterval
	}
	cfg, err := config.Load(configPathsFor(paths, opts), environment(), overrides)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return exitUsage
	}

	credentialStore := credentials.NewStore(credentials.Options{
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
	redactor := credentials.NewRedactor(token, credentialStore.EnvironmentToken())

	stateStore := d.stateStore
	if stateStore == nil {
		if opts.noCache {
			stateStore = cache.NewDisabledStore()
		} else {
			stateStore = cache.NewStore(paths.CacheFile)
		}
	}
	state := domain.EmptyStoredState()
	var cacheWarning *domain.Warning
	if !opts.noCache {
		state, err = stateStore.Load()
		if err != nil {
			state = domain.EmptyStoredState()
			warning := domain.Warning{Section: "Cache", Message: "ignored cache: " + redactor.Redact(err.Error())}
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
		snapshot := app.NewSnapshot(reading)
		if cacheWarning != nil {
			snapshot.Warnings = append(snapshot.Warnings, *cacheWarning)
		}
		if state.Samples == nil {
			state.Samples = make(map[domain.QuotaKey][]domain.Sample)
		}
		runway.RecordReading(state.Samples, reading)
		snapshot.Runways = runway.CalculateRunways(state.Samples, reading)
		state.CacheVersion = domain.CacheSchemaVersion
		state.Snapshot = &snapshot
		if !opts.noCache {
			if err := stateStore.Save(state); err != nil {
				snapshot.Warnings = append(snapshot.Warnings, domain.Warning{Section: "Cache", Message: "could not save state: " + redactor.Redact(err.Error())})
			}
		}
		return writeJSONProjection(stdout, stderr, snapshot, cfg.BaseURL, false, d.now, redactor)
	}

	if !opts.noCache && state.Snapshot != nil {
		snapshot := *state.Snapshot
		snapshot.Warnings = append([]domain.Warning{}, state.Snapshot.Warnings...)
		snapshot.Warnings = append(snapshot.Warnings, domain.Warning{Section: "Refresh", Message: redactor.Redact(fetchErr.Error())})
		return writeJSONProjection(stdout, stderr, snapshot, cfg.BaseURL, true, d.now, redactor)
	}
	if cacheWarning != nil {
		fmt.Fprintf(stderr, "kimiburn: cache: %s\n", cacheWarning.Message)
	}
	fmt.Fprintf(stderr, "kimiburn: %s\n", redactor.Redact(fetchErr.Error()))
	return exitFailure
}

func writeJSONProjection(stdout, stderr io.Writer, snapshot domain.Snapshot, baseURL string, cached bool, now func() time.Time, redactor *credentials.Redactor) int {
	if now == nil {
		now = time.Now
	}
	if err := app.WriteJSON(stdout, snapshot, app.ProjectionOptions{APIBase: baseURL, Cached: cached, Now: now()}); err != nil {
		message := err.Error()
		if redactor != nil {
			message = redactor.Redact(message)
		} else {
			message = credentials.Redact(message)
		}
		fmt.Fprintf(stderr, "kimiburn: %s\n", message)
		return exitFailure
	}
	return exitOK
}
