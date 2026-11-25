// Package kimiburn provides read-only, headless access to Kimi Code membership
// quotas and Kimiburn's local runway estimates.
//
// Callers supply credentials, HTTP dependencies, output writers, and optional
// in-memory state. The package never discovers environment credentials, opens a
// keyring or cache, or writes to process output streams. Raw provider data can
// contain private account information and must be kept out of logs and fixtures.
package kimiburn
