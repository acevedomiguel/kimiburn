# Kimiburn project instructions

Kimiburn is a read-only terminal monitor for Kimi Code membership quotas. It is
a sibling of Routeburn (`/Users/mike/projects/github/routeburn`) and adapts its
Go/Charm patterns, but no Routeburn or OpenRouter domain concept carries over.

## Product and credential boundaries

- Kimiburn is read-only. It must never send inference traffic, change quota,
  purchase anything, or create, rotate, or revoke a Kimi token.
- The only supported endpoint is `GET /usages` on `https://api.kimi.com/coding/v1`
  or the explicitly selected overseas `https://api.kimi.ai/coding/v1`. Never call
  both hosts automatically, and never fall back to Moonshot Open Platform
  (`https://api.moonshot.ai/...`) or OpenRouter APIs.
- The token is requested explicitly and resolved only from `KIMIBURN_TOKEN`, the
  OS keyring (`kimiburn`/`token`), or a restricted `token` file. Never read
  `KIMI_API_KEY`, Pi state, Kimi CLI state, or any other application's credential.
- Credentials and authorization headers are never exposed in any output,
  including logs, errors, JSON, cache, UI, tests, fixtures, or commits; they are
  stored only where the credential rules above allow. Route user-visible errors
  through `internal/credentials/redact.go`.
- Real account data is permitted only in the intended private local surfaces:
  the UI, `--json` output, and the local cache. Never put real account data in
  logs, errors, tests, fixtures, commits, or probe reports.
- Tests are synthetic-only: hand-authored fixtures, fake clocks/keyrings/
  transports, temporary directories, and `httptest`. They must not reach the real
  network, keyring, user configuration, user cache, or a real token, and must
  never commit a sanitized real response as a fixture.

## Architecture boundaries

- Module `github.com/acevedomiguel/kimiburn`, binary built from `./cmd/kimiburn`.
- `internal/app` and `internal/domain` must not import `internal/kimi`. The wire
  client maps responses into provider-neutral domain types behind `UsageSource`.
- Preserve missing-versus-zero: optional values stay pointers/null, never a
  fabricated `0`. Missing data is not zero, and an unavailable forecast is not
  infinite.
- Monthly membership (`limit_month_total`) and Kimi Code's monthly allocation
  (`limit_month_code`) are never added together; Code is an allocation inside the
  shared pool. Conflicting ratio/legacy representations surface as a conflict
  warning instead of an arbitrary winner.
- Unrecognized API fields round-trip through the internal cache and the public
  `--json` output. That raw data is local-only: it is never a log or commit
  payload.
- Public JSON schema and internal cache schema are versioned independently and
  both start at `1`.

## Terminal UI

- Minimum supported layout is 60x16; smaller terminals show a resize hint.
- Keep exact terminal height, no line overflow, a footer on the final row, and no
  ANSI escapes in no-color mode.
- Rendering stays pure: `internal/ui` never fetches, and `internal/app` holds
  state transitions and injected clocks.

## Baseline versions

- Go directive `1.26.0` in `go.mod`; CI also builds current stable Go.
- Charm v2 and supporting modules are pinned to the versions verified in
  Routeburn and adopted as their consumers land:
  `charm.land/bubbletea/v2 v2.1.2`, `charm.land/bubbles/v2 v2.2.2`,
  `charm.land/lipgloss/v2 v2.1.1`, `github.com/zalando/go-keyring v0.2.8`,
  `gopkg.in/yaml.v3 v3.0.1`.
- Commit messages are Conventional Commits; releases are derived from them, so
  subjects are lowercase and imperative.

## Verification

Run checks relevant to the changed area and report anything unavailable:

- Go changes: `make check`, plus `gofmt -l .` (no output), `go test -race ./...`,
  and `go build ./cmd/kimiburn` before a pull request.
- CLI/build changes: `make build` and `./kimiburn --version` without loading real
  credentials.
- Live probing is opt-in, requires an explicitly supplied token, and is never part
  of `go test`, CI, or a release snapshot.
