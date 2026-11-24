package app

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// PublicSchemaVersion is the stable schema-1 JSON contract. It is independent
// of the private cache and snapshot versions.
const PublicSchemaVersion = 1

// ProjectionOptions supplies transport-independent output metadata.
type ProjectionOptions struct {
	APIBase string
	Cached  bool
	Now     time.Time
}

type publicDocument struct {
	SchemaVersion   int                     `json:"schema_version"`
	GeneratedAt     time.Time               `json:"generated_at"`
	Cached          bool                    `json:"cached"`
	CacheAgeSeconds int64                   `json:"cache_age_seconds"`
	Source          publicSource            `json:"source"`
	Quotas          map[string]*publicQuota `json:"quotas"`
	GoodsVersion    json.RawMessage         `json:"goods_version"`
	BoosterWallet   json.RawMessage         `json:"booster_wallet"`
	Raw             json.RawMessage         `json:"raw"`
	Warnings        []domain.Warning        `json:"warnings"`
}

type publicSource struct {
	APIBase  string `json:"api_base"`
	Endpoint string `json:"endpoint"`
}

type publicQuota struct {
	Source         domain.QuotaSource `json:"source"`
	UsedRatio      *float64           `json:"used_ratio"`
	RemainingRatio *float64           `json:"remaining_ratio"`
	Limit          *float64           `json:"limit"`
	Used           *float64           `json:"used"`
	Remaining      *float64           `json:"remaining"`
	ResetAt        *time.Time         `json:"reset_at"`
	ResetInSeconds *int64             `json:"reset_in_seconds"`
	Runway         publicRunway       `json:"runway"`
}

type publicRunway struct {
	Status             string     `json:"status"`
	Reason             *string    `json:"reason"`
	Seconds            *float64   `json:"seconds"`
	ExhaustedAt        *time.Time `json:"exhausted_at"`
	BurnRatioPerSecond *float64   `json:"burn_ratio_per_second"`
	BurnPercentPerHour *float64   `json:"burn_percent_per_hour"`
	SampleCount        int        `json:"sample_count"`
	WindowSeconds      int64      `json:"window_seconds"`
	Confidence         string     `json:"confidence"`
	Basis              string     `json:"basis,omitempty"`
}

// WriteJSON emits only indented schema-1 JSON and a trailing newline. It
// marshals before writing so a projection error cannot leave partial stdout.
func WriteJSON(w io.Writer, snapshot domain.Snapshot, opts ProjectionOptions) error {
	generated := snapshot.GeneratedAt.UTC()
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	age := int64(0)
	if opts.Cached && now.After(generated) {
		age = int64(now.Sub(generated) / time.Second)
	}
	doc := publicDocument{
		SchemaVersion: PublicSchemaVersion, GeneratedAt: generated,
		Cached: opts.Cached, CacheAgeSeconds: age,
		Source:       publicSource{APIBase: opts.APIBase, Endpoint: "/usages"},
		Quotas:       make(map[string]*publicQuota, len(domain.CanonicalQuotaKeys)),
		GoodsVersion: snapshot.GoodsVersion, BoosterWallet: snapshot.BoosterWallet,
		Raw: snapshot.Raw, Warnings: append([]domain.Warning{}, snapshot.Warnings...),
	}
	for _, key := range domain.CanonicalQuotaKeys {
		quota, ok := snapshot.Quotas[key]
		if !ok {
			doc.Quotas[string(key)] = nil
			continue
		}
		doc.Quotas[string(key)] = projectQuota(quota, snapshot.Runways[key], generated)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("json: encode projection: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("json: write projection: %w", err)
	}
	return nil
}

func projectQuota(quota domain.QuotaReading, runway domain.RunwayEstimate, generated time.Time) *publicQuota {
	result := &publicQuota{
		Source: quota.Source, UsedRatio: quota.UsedRatio,
		Limit: quota.Limit, Used: quota.Used, Remaining: quota.Remaining,
		Runway: projectRunway(runway),
	}
	if quota.UsedRatio != nil && !math.IsNaN(*quota.UsedRatio) && !math.IsInf(*quota.UsedRatio, 0) && *quota.UsedRatio >= 0 && *quota.UsedRatio <= 1 {
		remaining := remainingRatio(*quota.UsedRatio)
		result.RemainingRatio = &remaining
	}
	if quota.ResetAt != nil {
		reset := quota.ResetAt.UTC()
		result.ResetAt = &reset
		seconds := int64(0)
		if reset.After(generated) {
			seconds = int64(reset.Sub(generated) / time.Second)
		}
		result.ResetInSeconds = &seconds
	}
	return result
}

// remainingRatio complements the shortest decimal representation of the
// reported ratio, avoiding binary subtraction artifacts such as
// 1-0.418 becoming 0.5820000000000001 in the public projection.
func remainingRatio(used float64) float64 {
	decimal, ok := new(big.Rat).SetString(strconv.FormatFloat(used, 'g', -1, 64))
	if !ok {
		return 1 - used
	}
	remaining, _ := new(big.Rat).Sub(big.NewRat(1, 1), decimal).Float64()
	return remaining
}

func projectRunway(runway domain.RunwayEstimate) publicRunway {
	result := publicRunway{
		Status: runway.Status, Seconds: runway.Seconds,
		BurnRatioPerSecond: runway.BurnRatioPerSecond, BurnPercentPerHour: runway.BurnPercentPerHour,
		SampleCount: runway.SampleCount, WindowSeconds: runway.WindowSeconds, Confidence: runway.Confidence, Basis: runway.Basis,
	}
	if runway.Reason != "" {
		reason := runway.Reason
		result.Reason = &reason
	}
	if runway.ExhaustedAt != nil {
		exhausted := runway.ExhaustedAt.UTC()
		result.ExhaustedAt = &exhausted
	}
	return result
}
