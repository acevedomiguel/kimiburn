package domain

import (
	"encoding/json"
	"time"
)

const (
	// SnapshotSchemaVersion versions the internal cached snapshot, not the
	// provider payload or the public one-shot JSON projection.
	SnapshotSchemaVersion = 1
	// CacheSchemaVersion versions the on-disk StoredState envelope.
	CacheSchemaVersion = 1
)

// Snapshot is the last successfully decoded live reading and its derived
// values. Raw provider objects remain structured JSON rather than strings.
type Snapshot struct {
	SchemaVersion int                         `json:"schema_version"`
	GeneratedAt   time.Time                   `json:"generated_at"`
	Quotas        map[QuotaKey]QuotaReading   `json:"quotas"`
	Runways       map[QuotaKey]RunwayEstimate `json:"runways"`
	GoodsVersion  json.RawMessage             `json:"goods_version"`
	BoosterWallet json.RawMessage             `json:"booster_wallet"`
	Raw           json.RawMessage             `json:"raw"`
	Warnings      []Warning                   `json:"warnings"`
}

// RunwayEstimate describes a local forecast. Numeric pointers distinguish an
// unavailable value from a genuine zero.
type RunwayEstimate struct {
	Status             string     `json:"status"`
	Seconds            *float64   `json:"seconds"`
	ExhaustedAt        *time.Time `json:"exhausted_at"`
	BurnRatioPerSecond *float64   `json:"burn_ratio_per_second"`
	BurnPercentPerHour *float64   `json:"burn_percent_per_hour"`
	SampleCount        int        `json:"sample_count"`
	WindowSeconds      int64      `json:"window_seconds"`
	Confidence         string     `json:"confidence"`
	Reason             string     `json:"reason"`
}

// Sample belongs to one quota's current reset period.
type Sample struct {
	ObservedAt time.Time `json:"observed_at"`
	UsedRatio  float64   `json:"used_ratio"`
	ResetAt    time.Time `json:"reset_at"`
}

// StoredState is the private cache envelope. Samples for each quota are
// independent; history activation is implemented by the runway slice.
type StoredState struct {
	CacheVersion int                   `json:"cache_version"`
	Snapshot     *Snapshot             `json:"snapshot"`
	Samples      map[QuotaKey][]Sample `json:"samples"`
}

// EmptyStoredState returns a usable cold-start state.
func EmptyStoredState() StoredState {
	return StoredState{CacheVersion: CacheSchemaVersion, Samples: make(map[QuotaKey][]Sample)}
}
