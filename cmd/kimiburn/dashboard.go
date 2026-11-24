package main

import (
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/acevedomiguel/kimiburn/internal/app"
	"github.com/acevedomiguel/kimiburn/internal/cache"
	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
)

func doDashboard(paths config.Paths, opts options, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
	noColor := opts.noColor
	if opts.command == "demo" {
		model := app.NewModel(app.Options{Demo: true, NoColor: noColor, Now: d.now})
		if d.runDashboard == nil {
			fmt.Fprintln(stderr, "kimiburn: interactive runner is unavailable")
			return exitFailure
		}
		if err := d.runDashboard(model, stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "kimiburn: dashboard: %s\n", earlyRedact(err.Error()))
			return exitFailure
		}
		return exitOK
	}

	overrides := config.Overrides{NoColor: opts.noColor}
	if opts.refreshSet {
		overrides.RefreshInterval = &opts.refreshInterval
	}
	cfg, err := config.Load(configPathsFor(paths, opts), environment(), overrides)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return exitUsage
	}
	noColor = noColor || cfg.Color == "never"
	credentialStore := credentials.NewStore(credentials.Options{Keyring: d.keyring, TokenFile: paths.TokenFile(), KeyringEnabled: cfg.Keyring})
	stateStore := d.stateStore
	if stateStore == nil {
		if opts.noCache {
			stateStore = cache.NewDisabledStore()
		} else {
			stateStore = cache.NewStore(paths.CacheFile)
		}
	}
	modelOptions, cancel, err := app.NewLiveModelOptions(app.LiveOptions{
		BaseURL: cfg.BaseURL, Interval: cfg.RefreshInterval, NoCache: opts.noCache, NoColor: noColor,
		Store: stateStore, ResolveToken: credentialStore.Resolve, NewUsage: d.newUsage, Now: d.now,
	})
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	defer cancel()
	modelOptions.Demo = false
	if d.runDashboard == nil {
		fmt.Fprintln(stderr, "kimiburn: interactive runner is unavailable")
		return exitFailure
	}
	if err := d.runDashboard(app.NewModel(modelOptions), stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "kimiburn: dashboard: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}
	return exitOK
}

func runTerminal(model app.Model, stdin io.Reader, stdout io.Writer) error {
	_, err := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout)).Run()
	return err
}
