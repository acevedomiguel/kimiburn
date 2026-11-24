# Security Policy

## Reporting a vulnerability

Do not open a public issue for a security problem. Use GitHub's private
vulnerability reporting on this repository (Security tab, “Report a
vulnerability”). If that is unavailable, contact the maintainer listed in
`LICENSE`.

Include the affected `kimiburn --version`, platform, reproduction steps, and
whether the issue is remotely triggerable. Replace every real token and account
response with an invented placeholder.

## Scope

In scope:

- Credential leakage into config, cache, JSON, logs, errors, tests, or crashes.
- Unsafe token/cache permissions, symlink or replacement behavior, and fallback
  writes that can silently lose a previously stored credential.
- Redaction failures, arbitrary production API hosts, untrusted redirects,
  unbounded response reads, or retry/auth behavior that hides a failure.
- Release workflow permissions, artifact substitution, and checksum integrity.

Out of scope:

- Vulnerabilities in Kimi's own service or an attack that already controls the
  user's shell/account or possesses the token.
- Missing hardening without demonstrated impact, such as certificate pinning.
- Quota, billing, membership, or rate-limit disputes handled by Kimi support.

## Security design

- **Read-only and narrow.** Production requests use only `GET /usages` beneath
  exactly one of two HTTPS Kimi Code bases. No inference, mutation, OAuth,
  Open Platform, or arbitrary-host mode is available.
- **Explicit credentials.** The token comes from `KIMIBURN_TOKEN`, the OS
  keyring, or Kimiburn's own fallback file. No other application's credentials
  are imported. Setup validates before saving; logout is local and does not
  revoke a token.
- **Restricted local state.** On POSIX-enforcing platforms private directories
  use `0700` and token/cache files `0600`. Fallback writes use a same-directory
  temporary file, close, and atomic platform replacement. Windows relies on the
  inherited user-profile ACL.
- **Redacted errors.** Only the `Authorization: Bearer` header carries the
  token. Response bodies become typed errors, and user-visible errors/warnings
  pass through credential redaction.
- **Private account data.** Raw JSON and cache deliberately retain provider
  fields, including unknown account-specific values. They are not credential
  stores, but they are still private and must never be committed or published
  without review.
- **Bounded I/O.** Each request has a 15-second attempt timeout and a 4 MiB
  response cap. At most two retries are allowed for transport errors,
  429/502/503/504, with bounded jitter and `Retry-After` handling. 401/403
  suspends automatic polling until a manual refresh succeeds.
- **Honest freshness.** Cached data and old runway estimates are labelled;
  cached/demo reads never append live samples.

If a token may be exposed, revoke it through the appropriate Kimi account
process, then run `kimiburn logout` and remove any private JSON/cache exports.
If raw account data may be exposed, delete those exports and rotate any secrets
they independently contain.

## Supported versions

Security fixes target the latest released minor version. Users of older
versions may be asked to upgrade before a fix is backported.
