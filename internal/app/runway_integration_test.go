package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

type usageSourceFunc func(context.Context) (domain.UsageReading, error)

func (f usageSourceFunc) ReadUsage(ctx context.Context) (domain.UsageReading, error) {
	return f(ctx)
}
func (f usageSourceFunc) NextRefreshAllowedAt() time.Time { return time.Time{} }

func TestLiveRunwayHistoryPersistsAndNoCacheStaysInMemory(t *testing.T) {
	for _, noCache := range []bool{false, true} {
		t.Run(map[bool]string{false: "persistent", true: "memory"}[noCache], func(t *testing.T) {
			start := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
			store := &memoryStateStore{state: domain.EmptyStoredState()}
			calls := 0
			opts, cancel, err := NewLiveModelOptions(LiveOptions{
				Store: store, NoCache: noCache,
				ResolveToken: func(context.Context) (string, error) { return "token", nil },
				NewUsage: func(string, string) (domain.UsageSource, error) {
					return usageSourceFunc(func(context.Context) (domain.UsageReading, error) {
						observed := start.Add(time.Duration(calls) * 30 * time.Second)
						calls++
						return domain.UsageReading{
							ObservedAt: observed, Raw: json.RawMessage(`{}`),
							Quotas: map[domain.QuotaKey]domain.QuotaReading{
								domain.QuotaLimit5h: {Source: "limit_5h", UsedRatio: ratioPointer(.2 + float64(calls-1)*.01), ResetAt: timePointer(start.Add(2 * time.Hour))},
							},
						}, nil
					}), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			var result RefreshResult
			for i := 0; i < 5; i++ {
				result = opts.Refresh(context.Background(), RefreshRequest{Generation: uint64(i + 1)})
				if result.Err != nil {
					t.Fatal(result.Err)
				}
			}
			estimate := result.Snapshot.Runways[domain.QuotaLimit5h]
			if estimate.Status != "finite" || estimate.SampleCount != 5 || estimate.WindowSeconds != 120 || estimate.Confidence != "low" {
				t.Fatalf("runway=%+v", estimate)
			}
			if noCache {
				if store.loads != 0 || store.saves != 0 {
					t.Fatalf("no-cache I/O: loads=%d saves=%d", store.loads, store.saves)
				}
			} else if store.saves != 5 || len(store.savedState.Samples[domain.QuotaLimit5h]) != 5 {
				t.Fatalf("saved samples=%+v saves=%d", store.savedState.Samples, store.saves)
			}
		})
	}
}

func ratioPointer(v float64) *float64    { return &v }
func timePointer(v time.Time) *time.Time { return &v }
