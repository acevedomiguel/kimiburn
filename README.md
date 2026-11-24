# Kimiburn

**A read-only terminal monitor for Kimi Code membership quotas and runway.**

```
                                     KIMIBURN · SYNTHETIC DATA · no provider calls
  ╭─ 5-hour quota ────────────────────────────────────────╮ ╭─ Monthly membership ───────────── Live · 14:32:30 UTC ─╮
  │ Remaining ▇▇▇▇▇▇▇▇▇▇░░░░░░░░░░░░░░░░░░░░░░░░░░░ 28.0% │ │ Remaining ▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇░░░░░░░░░░░░░░░░ 58.2% │
  │ Reset ····································· in 3h 12m │ │ Reset ······································ in 18d 6h │
  │ Runway ······································· 2h 20m │ │ Runway ····························· Beyond this reset │
  │ Burn ········································ 12.0%/h │ │ Burn ······································· 0.90%/day │
  ╰───────────────────────────────────────────────────────╯ ╰────────────────────────────────────────────────────────╯
  ╭─ 7-day quota · legacy ───────────────────────────────────────────────────────────────────────────────────────────╮
  │ Remaining ▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ 60.0% │
  │ Reset ································································································· in 2d 0h │
  │ Runway ················································································· No consumption observed │
  │ Burn ································································································· 0.00%/day │
  │ Requests ················································································· 40/100 used · 60 left │
  ╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯

 [r] refresh · [↑↓/j/k] scroll · [pgup/pgdn] page · [?] help · [q] quit
```

Illustrative rendering with synthetic values, reproduced locally with
`kimiburn demo`. The dashboard shows only your own account figures.

Kimiburn shows the shared monthly membership, the Monthly Code allocation when
reported, the rolling 5-hour quota, and a legacy 7-day quota when the API
reports one. It also derives local burn-rate and runway estimates from
repeated observations.

Built with Go and Charm as a sibling of
[Routeburn](https://github.com/acevedomiguel/routeburn).

> Kimiburn only reads data. It sends no inference traffic, changes no quota,
> never rotates or revokes tokens, and never falls back to Moonshot Open
> Platform. Ratio-only responses cannot reveal absolute request, token, or
> currency amounts. The monthly Code allocation is part of the shared
> membership quota; the two are never added together.

## What it does

- Reads only `GET /usages` beneath the selected Kimi Code API base.
- Refreshes every 30 seconds by default, measured from request completion.
- Preserves unknown provider fields in local `--json` output.
- Keeps a private, versioned cache for offline display and local samples.
- Labels cached data, parser conflicts, missing values, and unavailable
  estimates instead of inventing plausible numbers.
- Includes a deterministic synthetic demo that needs no token or network.

## Installation

### Homebrew (macOS and Linux)

```bash
brew tap acevedomiguel/kimiburn
brew install --cask kimiburn
kimiburn --version
```

Upgrades arrive through the same tap:

```bash
brew update
brew upgrade --cask kimiburn
```

The cask downloads a prebuilt binary rather than compiling from source. That
binary is not notarized by Apple, so the cask clears the macOS quarantine flag
after staging; without that step Gatekeeper would kill the binary on first run.
The tap is a third-party tap, so Homebrew asks you to trust it once:

```bash
brew trust acevedomiguel/kimiburn
```

### Go

```bash
go install github.com/acevedomiguel/kimiburn/cmd/kimiburn@latest
```

### Prebuilt binaries

Every release publishes Linux, macOS, and Windows archives plus a
`checksums.txt` file on the GitHub Releases page. Archives contain the binary,
this README, and the LICENSE; Linux and macOS use `.tar.gz`, Windows uses
`.zip`. Targets are Linux/macOS/Windows amd64 and Linux/macOS arm64. Verify a
download with:

```bash
sha256sum -c checksums.txt --ignore-missing
```

### From source

Use Go 1.26.0 or newer:

```bash
git clone https://github.com/acevedomiguel/kimiburn.git
cd kimiburn
make build
./kimiburn --version
```

## Set up a token

Supply a Kimi Code token explicitly. Kimiburn never imports credentials from
Pi, the Kimi CLI, or another application.

```sh
kimiburn setup
```

Running `kimiburn` without a token starts this setup automatically, then opens
the dashboard after success; a second command is not needed. Setup accepts a
hidden terminal entry or one piped token, validates it with one live usage
request, and then stores it. The OS keyring is preferred. If the
keyring is unavailable, a permission-restricted fallback file is used. For a
non-interactive process, set `KIMIBURN_TOKEN` in its environment.

```sh
printf '%s\n' "$KIMIBURN_TOKEN" | kimiburn setup
```

Do not paste tokens into shell history, issues, logs, or commits. `logout`
removes only Kimiburn's local token and keyring entry; it does not revoke the
token with Kimi:

```sh
kimiburn logout
```

## Use

```sh
kimiburn                    # live alternate-screen dashboard
kimiburn demo --no-color    # synthetic dashboard; no credentials or network
kimiburn --json             # one live schema-1 JSON document on stdout
kimiburn --json --no-cache  # no cache reads or writes
kimiburn --refresh 45s      # override the polling interval
kimiburn --refresh 0        # manual refresh only
kimiburn --help
kimiburn --version
```

Dashboard keys: `r` refreshes, arrow keys or `j`/`k` scroll, Page Up/Page Down
scroll a page, Home/End jump, `?` opens help, and `q` quits. A manual refresh is
throttled to one attempt per 30 seconds. A 401/403 suspends automatic requests
until a manual refresh succeeds. Refresh intervals below 30 seconds are
rejected unless the value is zero. Demo mode uses the same 30-second scheduler
but refreshes only synthetic data and never contacts the provider.

### Panels and runway

The monthly membership and 5-hour panels have priority. Monthly Code allocation
(`limit_month_code`) appears only when reported; it is included in the monthly
membership, not an additional allowance. Legacy 7-day data also appears only
when reported. Each quota row places a Remaining label, proportional bar, and
remaining percentage in separate columns. The dashboard keeps raw provider
JSON out of the way; use `--json` when you need the complete structured
response.

Runway and burn are calculated locally, not supplied by Kimi. On a first read,
Kimiburn estimates average burn from the reported ratio and elapsed portion of
the current fixed 5-hour or 7-day window. Monthly estimates assume the reset
repeats on the same monthly date, clamped to the previous month's last day.
After at least five local samples spanning 120 seconds, ordinary least squares
uses the observed sample trend instead. JSON reports the estimator as
`window_average` or `local_samples` in each runway's `basis` field.

A reset or ratio decrease clears only the affected quota's history. Forecasts
never cross reset: `Beyond this reset` has no exhaustion timestamp. Missing
resets, invalid ratios, and conflicting sources produce explicit unavailable
reasons.

## Configuration

```sh
kimiburn config path
kimiburn config reset
```

See [`config.example.yaml`](config.example.yaml). Precedence is command-line
flags, `KIMIBURN_*` environment variables, strict YAML, then defaults. Unknown
YAML keys, extra YAML documents, arbitrary API hosts, and token-looking config
keys are rejected.

Defaults:

```yaml
version: 1
refresh_interval: 30s
base_url: https://api.kimi.com/coding/v1
theme: auto
color: auto
keyring: true
```

The only other accepted base is `https://api.kimi.ai/coding/v1`, selected
explicitly for an overseas account. There is no automatic regional fallback.

Environment variables: `KIMIBURN_TOKEN`, `KIMIBURN_REFRESH_INTERVAL`,
`KIMIBURN_BASE_URL`, `KIMIBURN_THEME`, `KIMIBURN_COLOR`, and
`KIMIBURN_KEYRING`. Global flags include `--config`, `--json`, `--refresh`,
`--no-cache`, and `--no-color`.

## JSON, cache, and privacy

`--json` writes exactly one schema-1 document to stdout. It includes normalized
quota entries, nullable local runway values, warnings, freshness metadata, and
the raw provider object. A failed fetch with a valid cache emits cached JSON
with a warning and exit 0. A failed fetch without cache emits no JSON and exits
1. Missing credentials exit 3; usage errors exit 2.

The cache and raw JSON can contain account-specific API data. They are local
private outputs, not safe fixtures or public logs. Review them before sharing.
Tokens are not written to cache or JSON. Errors and warnings redact configured
credentials.

Typical paths (platform defaults and absolute XDG overrides apply):

- Config: `${XDG_CONFIG_HOME:-~/.config}/kimiburn/config.yaml`
- Fallback token: `${XDG_CONFIG_HOME:-~/.config}/kimiburn/token`
- Cache/samples: `${XDG_CACHE_HOME:-~/.cache}/kimiburn/state.json`

On platforms where POSIX permissions are enforced, private directories are
`0700` and token/cache files are `0600`. Windows uses the inherited user-profile
ACL rather than POSIX permission bits.

To uninstall, remove the downloaded binary (or run `brew uninstall --cask
kimiburn` for a Homebrew install), run `kimiburn logout`, and delete
the config and cache directories shown above. No website, OAuth
flow, hosted service, or Open Platform integration is currently provided.

## API stability and limitations

The endpoint and response variants are based on official Kimi client behavior,
not a public stability guarantee. See [`docs/api-contract.md`](docs/api-contract.md)
for mappings and evidence. Exact account behavior remains subject to an opt-in
redacted live probe; no real account response is committed.

## Development

```sh
make check
# equivalent minimum gate:
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build ./cmd/kimiburn
```

Tests are synthetic-only and require no token, network, keyring, or real user
state. The rendering at the top of this file is an illustration with synthetic
values, not a recorded account; `kimiburn demo` reproduces it without a token
or network. See [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`SECURITY.md`](SECURITY.md). Licensed under MIT.
