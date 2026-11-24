# Contributing to Kimiburn

## Development setup

- Go 1.26.0 or newer; `go.mod` declares the minimum and CI also tests current
  stable Go.
- No other runtime dependency is required yet.

```bash
go build ./cmd/kimiburn
go test ./...
```

Useful Makefile targets: `make build`, `make test`, `make fmt`, `make vet`,
`make check`.

## Testing expectations

- Test-first: write the failing test, watch it fail, then make it pass.
- Tests are synthetic-only. Use fake clocks, transports, and keyrings plus
  `httptest`; never call the real network, keyring, user cache, or user config.
- Fixtures are hand-authored. Never commit a real API response, even a sanitized
  or truncated one, and never record real ratios, reset timestamps, or wallet
  values.
- Domain calculations are pure functions with injected clocks so results are
  deterministic.

## Commit messages

Releases are derived from Conventional Commits, so the format is load-bearing
rather than cosmetic.

- `fix: ...` produces a patch release.
- `feat: ...` produces a minor release.
- `feat!: ...` or a `BREAKING CHANGE:` footer produces a major release.
- `docs:`, `chore:`, `ci:`, `test:`, and `style:` produce no release.

Subjects are lowercase, imperative, and under 72 characters, for example
`feat: add Kimi quota API client`.

## Credentials

Never commit a token. Kimiburn reads `KIMIBURN_TOKEN` for non-interactive use or
a token you store explicitly with `kimiburn setup`. It does not import
credentials from Pi, the Kimi CLI, or any other tool. Do not paste real account
responses into issues, pull requests, or review comments.

## Read-only boundary

Contributions must not add behavior that sends inference traffic, mutates quota,
or calls Open Platform or OpenRouter endpoints. Keep new API calls on the single
documented `GET /usages` endpoint.

## License

Contributions are accepted under the MIT license in `LICENSE`.
