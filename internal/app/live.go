package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/acevedomiguel/burnblocks/kimi"
)

// LiveOptions describes the production adapters without coupling the
// interactive model to a concrete credential, HTTP, or filesystem package.
type LiveOptions struct {
	BaseURL       string
	Interval      time.Duration
	NoCache       bool
	NoColor       bool
	Store         kimi.StateStore
	ResolveToken  func(context.Context) (string, error)
	NewUsage      func(baseURL, token string) (kimi.UsageSource, error)
	IsAuthFailure func(error) bool
	Now           func() time.Time
}

type nextRefreshSource interface{ NextRefreshAllowedAt() time.Time }

// NewLiveModelOptions composes cache discovery and one-request-at-a-time live
// reads. Samples are preserved for the runway layer; failed reads never erase
// the last good snapshot. No token is requested until the model initializes.
func NewLiveModelOptions(live LiveOptions) (Options, context.CancelFunc, error) {
	if live.Now == nil {
		live.Now = time.Now
	}
	if live.Store == nil {
		live.Store = kimi.NewDisabledStateStore()
	}
	state := kimi.EmptyStoredState()
	var snapshot kimi.Snapshot
	warning := ""
	if !live.NoCache {
		loaded, err := live.Store.Load()
		if err != nil {
			warning = "ignored cache: " + kimi.Redact(err.Error())
		} else {
			state = loaded
			if state.Samples == nil {
				state.Samples = make(map[kimi.QuotaKey][]kimi.Sample)
			}
			if state.Snapshot != nil {
				snapshot = *state.Snapshot
			}
		}
	}
	_, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	opts := Options{
		Snapshot: snapshot, Warning: warning, History: cloneHistory(state.Samples),
		Interval: live.Interval, IntervalSet: true, NoColor: live.NoColor,
		Now: live.Now, Cancel: cancel,
	}
	opts.Configured = func() (bool, error) {
		if live.ResolveToken == nil {
			return false, errors.New("credential resolver is unavailable")
		}
		token, err := live.ResolveToken(context.Background())
		if err != nil {
			return false, errors.New(kimi.Redact(err.Error()))
		}
		return token != "", nil
	}
	opts.Refresh = func(requestCtx context.Context, request RefreshRequest) RefreshResult {
		result := RefreshResult{Generation: request.Generation, Manual: request.Manual}
		if live.ResolveToken == nil {
			result.Err = errors.New("credential resolver is unavailable")
			return result
		}
		token, err := live.ResolveToken(requestCtx)
		if err != nil {
			result.Err = errors.New(kimi.Redact(err.Error()))
			return result
		}
		if token == "" {
			result.AuthFailure = true
			result.Err = errors.New("no Kimi Code token is configured; run kimiburn setup")
			return result
		}
		if live.NewUsage == nil {
			result.Err = errors.New("usage reader is unavailable")
			return result
		}
		redactor := kimi.NewRedactor(token)
		source, err := live.NewUsage(live.BaseURL, token)
		if err != nil {
			result.Err = errors.New(redactor.Redact(err.Error()))
			return result
		}
		reading, fetchErr := source.ReadUsage(requestCtx)
		if constrained, ok := source.(nextRefreshSource); ok {
			result.NextAllowed = constrained.NextRefreshAllowedAt()
		}
		if fetchErr != nil {
			if live.IsAuthFailure != nil {
				result.AuthFailure = live.IsAuthFailure(fetchErr)
			}
			message := redactor.Redact(fetchErr.Error())
			if message == fetchErr.Error() {
				result.Err = fetchErr
			} else {
				result.Err = errors.New(message)
			}
			return result
		}
		if reading.ObservedAt.IsZero() {
			reading.ObservedAt = live.Now()
		}
		result.Snapshot = kimi.BuildSnapshot(reading, &state)
		mu.Lock()
		defer mu.Unlock()
		result.History = cloneHistory(state.Samples)
		state.Snapshot = &result.Snapshot
		if live.NoCache {
			return result
		}
		if err := live.Store.Save(state); err != nil {
			result.Snapshot.Warnings = append(result.Snapshot.Warnings, kimi.Warning{Section: "Cache", Message: "could not save state: " + redactor.Redact(err.Error())})
		}
		return result
	}
	return opts, cancel, nil
}

// cloneHistory copies the sample history so the interactive model never shares
// a slice with the refresh goroutine that keeps appending to it.
func cloneHistory(history map[kimi.QuotaKey][]kimi.Sample) map[kimi.QuotaKey][]kimi.Sample {
	out := make(map[kimi.QuotaKey][]kimi.Sample, len(history))
	for key, samples := range history {
		out[key] = append([]kimi.Sample(nil), samples...)
	}
	return out
}
