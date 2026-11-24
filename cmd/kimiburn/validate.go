package main

import (
	"context"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/kimi"
)

// validateToken returns the production setup validator: exactly one bounded
// usage read against the configured base URL. It resolves configuration the
// same way setup does, so the token is checked against the host it will be
// stored for. A 2xx top-level object validates the token even when no quota is
// reported.
func validateToken(configPath string) func(context.Context, string) error {
	return func(ctx context.Context, token string) error {
		paths, err := config.DefaultPaths()
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPathsFor(paths, options{configPath: configPath}), environment())
		if err != nil {
			return err
		}
		return readUsageOnce(ctx, cfg.BaseURL, token, version)
	}
}

// readUsageOnce constructs the bounded client and performs one read. The client
// owns its own retry policy; this function adds no request of its own.
func readUsageOnce(ctx context.Context, baseURL, token, clientVersion string) error {
	client, err := kimi.NewClient(
		kimi.WithBaseURL(baseURL),
		kimi.WithToken(token),
		kimi.WithVersion(clientVersion),
	)
	if err != nil {
		return err
	}
	_, err = client.ReadUsage(ctx)
	return err
}
