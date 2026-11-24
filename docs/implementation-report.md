# Initial implementation report

Date: 2026-11-24
Branch: `feat/initial-kimiburn-monitor`
Approved plan: `2026-11-24-kimiburn-initial-design`

## Scope delivered

Kimiburn is a public, read-only Go/Charm terminal application for Kimi Code
quota monitoring. It includes explicit-token setup/logout, strict config,
bounded Kimi Code usage HTTP access, tolerant modern/legacy decoding, raw-field
retention, schema-1 JSON, a private atomic cache, a responsive alternate-screen
TUI, a synthetic demo, completion-plus-30-second polling, cached fallback, and
reset-bounded local runway estimates. Cross-platform CI and automated
GoReleaser/semantic-release configuration are included.

No website, Homebrew publication, OAuth import, Open Platform integration,
inference call, or real account fixture was added.

## Task-to-commit map

| Task | Evidence |
| --- | --- |
| S1.1 repository baseline | `3462768` (baseline `e7113ba` plus corrected data rules) |
| S1.2 strict config and paths | `d6fd5b0` and preceding `3031272`, `792007a`, `d8c1983` |
| S1.3 credentials and redaction | `1276f76` and preceding `8c15eb5`, `4049cc8` |
| S1.4 bounded usage client | `6164cde` and preceding `8904099`, `c6f49d2` |
| S1.5 modern/legacy decoding | `64d8fd2` and preceding `45d1613`, `ad79a14` |
| S1.6 atomic versioned cache | `fd49391` |
| S1.7 stable JSON and one-shot CLI | `9a020a7` |
| S1.8 compatibility contract | `a903126` |
| S1.9 authenticated redacted probe | **Blocked:** no explicit live user token was supplied; see residual risks |
| S2.1 deterministic synthetic demo | `84ee54a` |
| S2.2 quota panels and responsive layout | `84ee54a` |
| S2.3 interactive dashboard states/help | `a2460fd` |
| S2.4 completion-based scheduler/auth suspension | `a2460fd` |
| S2.5 live/cache/TUI composition | `bfd5c23` |
| S2.6 help, README, and CLI behavior | `8a31c4c` |
| S2.7 UI/race/build verification | `8a31c4c`; command evidence below |
| S3.1 sample history and persistence | `065670f` |
| S3.2 OLS runway and confidence | `065670f` |
| S3.3 live/JSON/restart integration | `065670f` |
| S3.4 help and limitation language | `065670f` |
| S3.5 deterministic runway scenarios | `065670f` |
| S4.1 public docs and safety review | `8a31c4c` |
| S4.2 cross-platform CI and secret checks | `8a31c4c` |
| S4.3 automated cross-platform releases | `8a31c4c`; local GoReleaser evidence below |
| S4.4 clean-checkout/privacy smoke | `8a31c4c`; clean-clone evidence below |
| S4.5 acceptance and PR handoff | this report and the feature pull request; no version manually published |

## Changed files

The initial branch changes 92 files relative to the empty repository baseline.
The complete reviewable list is available with:

```sh
git diff --name-only main...feat/initial-kimiburn-monitor
```

Areas changed:

- `cmd/kimiburn/`: CLI dispatch, setup/logout/config, one-shot JSON, dashboard.
- `internal/config/`, `internal/credentials/`, `internal/cache/`: strict local
  configuration, owned credential lifecycle, atomic private state.
- `internal/kimi/`, `internal/domain/`: bounded transport, tolerant decoding,
  canonical quota/raw models.
- `internal/app/`, `internal/runway/`: snapshots, Bubble Tea states, polling,
  cache composition, sample history, OLS runway and confidence.
- `internal/ui/`: pure formatting, synthetic fixtures, panels and responsive
  geometry.
- Root docs/config: README, SECURITY, CONTRIBUTING, API and implementation
  contracts, sample config, Go module and Makefile.
- `.github/`, `.goreleaser.yaml`, `.releaserc.json`, `package*.json`,
  `scripts/`: CI, credential checks, Conventional Commits and releases.

No Routeburn file is included in this branch. No cache, token, probe output,
`node_modules`, build output, or real account data is tracked.

## Tests and verification

The branch contains 203 top-level Go test functions, plus table-driven
subtests. Tests use fake clocks, keyrings, transports, sources, and state
stores. No live token or provider response is required.

The following passed in the implementation worktree at `8a31c4c`:

```sh
test -z "$(gofmt -l .)"
go vet ./...
go test -race -count=1 ./...
go build ./cmd/kimiburn
GOOS=windows GOARCH=amd64 go build ./cmd/kimiburn
```

The same minimum gate, `go mod tidy` with no diff, GoReleaser validation and
snapshot, archive inspection, checksum verification, empty-environment
synthetic demo/help/version smoke, and credential/private-state scans then
passed from a fresh local clone of `8a31c4c`.

Release evidence:

- `goreleaser check` succeeded with GoReleaser v2.20.3.
- `goreleaser release --snapshot --clean` built five targets: Linux
  amd64/arm64, macOS amd64/arm64, and Windows amd64. Windows arm64 is excluded.
- Every archive contains the binary, README, and LICENSE. `checksums.txt`
  verified all five SHA-256 entries.
- The native snapshot printed `kimiburn version 0.0.0-SNAPSHOT-8a31c4c`.
- `npm ci --offline --ignore-scripts`, `npm run check:release-notes`, and
  commitlint succeeded with Node v26.13.1. CI uses normal `npm ci`.
- The synthetic demo rendered with an empty environment/HOME under a pseudo-TTY
  and exited without provider access.

## Residual risks and explicit limitations

1. **S1.9 remains unchecked.** No authenticated probe was run because the user
   did not explicitly supply a live token. Exact account wire variant, complete
   observed keys/types, timestamp precision, and wallet shape remain unverified
   against a real account. Official-client evidence and synthetic variants
   guide the parser, but are not a public API stability guarantee.
2. Ratio-only responses cannot supply absolute request/token/currency totals.
   Shared monthly membership and monthly Code allocation are not additive.
3. Local runway quality depends on provider update frequency and user traffic.
   Confidence labels, reset clearing, and beyond-reset handling do not eliminate
   statistical uncertainty.
4. No official polling SLA was established. The default remains 30 seconds;
   client backoff and auth suspension are implemented.
5. OS keyring and Windows ACL behavior are covered by fakes/builds and inherited
   platform patterns, not by a live multi-OS credential integration run here.
   CI is configured to execute the full gate on Linux, macOS, and Windows.
6. Raw JSON/cache may contain account-specific values. They are intentionally
   local-only private outputs and must not be committed or published.

## Handoff

Open a pull request from `feat/initial-kimiburn-monitor` to `main`. CI and
commit-title checks run on the PR. After review and merge, semantic-release on
`main` determines whether a version is warranted and invokes GoReleaser. Do not
manually tag or publish the initial version as part of this handoff.
