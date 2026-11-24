// Package domain holds Kimiburn's provider-neutral types. It never imports a
// wire client, so the application and UI depend on this package only.
package domain

import (
	"context"
	"encoding/json"
	"time"
)

// UsageSource is the domain-facing boundary the composition root fills with a
// Kimi client. NextRefreshAllowedAt reports the earliest instant another read
// may run, or the zero time when no constraint was observed.
type UsageSource interface {
	ReadUsage(context.Context) (UsageReading, error)
	NextRefreshAllowedAt() time.Time
}

// UsageReading is one successful provider reading. Optional values stay
// pointers or null so missing data is never confused with zero. Quotas remains
// empty until the tolerant decoder populates it.
type UsageReading struct {
	ObservedAt    time.Time
	Quotas        map[QuotaKey]QuotaReading
	GoodsVersion  json.RawMessage
	BoosterWallet json.RawMessage
	Raw           json.RawMessage
	Warnings      []Warning
}

// QuotaKey names one canonical quota. Monthly membership and Kimi Code's
// monthly allocation are separate keys and are never added together.
type QuotaKey string

const (
	QuotaLimit5h         QuotaKey = "limit_5h"
	QuotaLimit7d         QuotaKey = "limit_7d"
	QuotaLimitMonthTotal QuotaKey = "limit_month_total"
	QuotaLimitMonthCode  QuotaKey = "limit_month_code"
)

// QuotaSource records how a quota's numbers were obtained. It is empty until
// the decoder assigns one.
type QuotaSource string

const (
	SourceRatio        QuotaSource = "ratio"
	SourceLegacyCounts QuotaSource = "legacy_counts"
)

// ConflictReason marks a quota whose recognized representations disagree or
// cannot be reconciled, so downstream estimation must fail closed rather than
// choose a winner. The empty value means no conflict was observed.
type ConflictReason string

// ConflictSources marks a quota whose modern ratio and legacy counts describe
// the same window with a material disagreement, or a modern ratio that is
// present but unusable while legacy counts exist. It is the stable value
// downstream runway estimation checks to refuse a normalized result.
const ConflictSources ConflictReason = "conflicting_sources"

// QuotaReading is one quota's normalized values. A nil pointer means the
// provider did not report that value; it never means zero.
type QuotaReading struct {
	Source         QuotaSource
	UsedRatio      *float64
	Limit          *float64
	Used           *float64
	Remaining      *float64
	ResetAt        *time.Time
	ConflictReason ConflictReason
	Raw            json.RawMessage
}

// Warning is a stable section name plus a human-readable message that has
// already passed through the redactor.
type Warning struct {
	Section string
	Message string
}
