# Kimiburn

A read-only terminal monitor for Kimi Code membership quotas, built as a sibling
of [Routeburn](https://github.com/acevedomiguel/routeburn).

Kimiburn answers three questions without sending inference traffic: how much
quota is left, when it resets, and whether current consumption will run out
first. It reads only `GET /usages` from the Kimi Code API every 30 seconds and
keeps every returned field available for local JSON inspection.

## Status

Early development. The repository currently ships the CLI skeleton
(`--help`/`--version`); quota fetching, the dashboard, and runway estimation are
implemented in later slices of the approved plan.

## Build

```bash
go build ./cmd/kimiburn
./kimiburn --version
```

Or use the Makefile: `make build`, `make test`, `make check`.

## Boundaries

- Read-only: no inference requests, no quota changes, no token rotation.
- One endpoint, one token you supply explicitly; no credential is imported from
  Pi, the Kimi CLI, or Moonshot Open Platform.
- `--json` and the local cache can contain your full raw API response. Treat that
  output as private and never commit it.

See `AGENTS.md` for engineering contracts and `CONTRIBUTING.md` for the
contribution workflow. Licensed under MIT (`LICENSE`).
