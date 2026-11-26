# Kimi Code usage API compatibility contract

Kimiburn is an independent, read-only client. It is not affiliated with Kimi.
This document describes compatibility with behavior observed in official Kimi
clients; it is not a stable public API specification.

The calling implementation — the bounded client, tolerant modern/legacy
decoding, and schema-1 projection — now lives in
[Burn Blocks](https://github.com/acevedomiguel/burnblocks) as
`github.com/acevedomiguel/burnblocks/kimi`. This document remains here as the
provider-compatibility record that implementation follows.

## Endpoint and authentication

Kimiburn calls `GET {base}/usages` with the configured Kimi Code bearer token.
The only production bases are:

- `https://api.kimi.com/coding/v1`
- `https://api.kimi.ai/coding/v1`

The base is selected explicitly. Kimiburn never falls back from one regional
base to the other, redirects a bearer token, calls an inference endpoint, or
uses an OpenRouter endpoint. The official Kimi Code managed-usage client is the
evidence for this request shape [1].

The endpoint is not documented as a stable public integration contract. Field
availability and response shapes may change. Kimiburn therefore keeps parsing
tolerant, fails closed on conflicting quota representations, and preserves the
complete provider object for local inspection.

## Supported response mappings

The mappings below come from the official Kimi Code managed-usage client and
its tests [1][2], the official membership documentation [3], and the official
Kimi CLI usage presentation [4].

| Provider data | Kimiburn interpretation | Evidence |
| --- | --- | --- |
| `usages.limit_5h` | Current rolling five-hour quota | [1][2][3] |
| `usages.limit_7d` | Legacy seven-day quota, when supplied | [1][2][4] |
| `usages.limit_month_total` | Shared monthly membership quota | [1][2][3] |
| `usages.limit_month_code` | Monthly allocation specific to Kimi Code | [1][2][3] |
| `used_ratio` | Fraction of the corresponding quota already consumed | [1][2] |
| `reset_time` | Reset instant for the corresponding quota | [1][2] |
| `goods_version` | Opaque plan/version value passed through as JSON | [1][2] |
| `boosterWallet` | Opaque additional account data passed through as JSON | [1][2] |
| Legacy `usage` and `limits[]` entries | Compatibility input for older count-based responses | [2][4] |
| Legacy `window.duration` and `window.timeUnit` | Window identification for supported legacy entries | [2][4] |
| Legacy `detail`, `limit`, and `remaining` | Count-based values used only when they reconcile safely | [2][4] |

`limit_month_total` and `limit_month_code` are separate views. Kimiburn never
adds them together [1][3].

Modern ratio-only responses do not contain an absolute request count. Kimiburn
therefore leaves `limit`, `used`, and `remaining` null unless supported legacy
counts provide them. It never converts ratios into invented token, currency,
or request quantities.

Newer plans use rolling five-hour limits. Legacy seven-day and count-based
representations are conditional compatibility paths, not fields every account
will return [1][3][4].

## Unknown or intentionally uninterpreted data

The following are unknown and remain so in Kimiburn's normalized model:

- the meaning and units of values inside `boosterWallet`;
- the complete set of `goods_version` values;
- whether a particular account will expose modern, legacy, or additional
  fields;
- undocumented fields, nested values, and future error-envelope members.

Unknown values are retained under `raw` in local JSON and cache output. A new
provider field does not require a public Kimiburn JSON schema change. The
absence of a field is not treated as zero.

## History and runway are local estimates

The usage response provides current quota and reset information, not a
historical burn series or a server-computed exhaustion forecast. For a first
reading, Kimiburn derives average burn from the elapsed portion of the current
5-hour or 7-day window. For these rolling quotas, once at least five samples
spanning 120 seconds exist, local ordinary least squares replaces this initial
average. Monthly estimates always use the elapsed current monthly window,
whose start is the same reset date in the previous month, clamped to that
month's last day. Public JSON distinguishes these methods with `basis` values
`window_average` and `local_samples`.

Neither method implies that Kimi supplied the estimate. Reset changes and
ratio decreases invalidate the old series; no forecast may cross a reset
boundary.

## Raw-data privacy

Provider responses can contain account-specific data. Raw values are allowed
only in the requesting user's explicit JSON output and private local cache.
They must not appear in logs, error strings, issue
reports, committed fixtures, screenshots, or documentation examples.

Repository fixtures and examples use invented synthetic values. They are not
redacted captures and contain no real account timestamps, identifiers, quota
values, wallet values, or tokens. A live contract probe is opt-in and requires
an explicitly supplied token; durable probe output must use a redacted schema,
not a raw response.

## Moonshot Open Platform is a different product

Moonshot Open Platform balance or usage APIs are not substitutes for the Kimi
Code membership usage endpoint. Kimiburn does not call them, map their balance
to a Kimi Code quota, or use them as a fallback. Open Platform and Kimi Code
credentials and products must not be conflated.

## Official evidence

1. [Kimi Code managed usage client](https://github.com/MoonshotAI/kimi-code/blob/main/packages/oauth/src/managed-usage.ts)
2. [Kimi Code managed usage tests](https://github.com/MoonshotAI/kimi-code/blob/main/packages/oauth/test/managed-usage.test.ts)
3. [Kimi Code membership documentation](https://www.kimi.com/code/docs/en/kimi-code/membership)
4. [Kimi CLI usage presentation](https://github.com/MoonshotAI/kimi-cli/blob/main/src/kimi_cli/ui/shell/usage.py)

These links document the compatibility evidence, not a promise of endpoint
stability or endorsement by Kimi.
