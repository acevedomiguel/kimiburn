package app

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ScheduleDelay always returns a timer duration. In particular, a delayed
// retry still arms a timer instead of disabling all future automatic polls.
func (m Model) ScheduleDelay(now time.Time, interval time.Duration) (time.Duration, bool) {
	delay := maxDuration(0, interval)
	if !m.nextAllowed.IsZero() {
		delay = maxDuration(delay, m.nextAllowed.Sub(now))
	}
	allowed := !m.inFlight && !m.suspended && m.state != ViewSetup && m.state != ViewFatal
	return delay, allowed
}

// StartAttempt commits throttle and generation state before returning the
// asynchronous command. A zero automatic interval never disables manual waits.
func (m *Model) StartAttempt(manual bool) (tea.Cmd, bool) {
	if m.inFlight || m.state == ViewSetup || m.state == ViewFatal || (!manual && m.suspended) {
		return nil, false
	}
	now := m.now()
	minimum := m.interval
	if manual {
		minimum = 30 * time.Second
	}
	if m.lastAttempt.IsZero() {
		m.lastAttempt = now.Add(-minimum)
	}
	required := m.lastAttempt.Add(minimum)
	if m.nextAllowed.After(required) {
		required = m.nextAllowed
	}
	if now.Before(required) {
		m.nextAllowed = required
		return nil, false
	}

	m.lastAttempt, m.nextAllowed = now, now.Add(minimum)
	m.generation++
	if manual {
		m.manual++
		m.manualSuccess = false
	}
	generation := m.generation
	m.inFlight, m.notice = true, ""
	if m.snapshot.GeneratedAt.IsZero() {
		m.state = ViewLoading
	}
	request := RefreshRequest{Generation: generation, Manual: manual}
	refresh := m.refresh
	ctx, cancelAttempt := context.WithCancel(context.Background())
	m.cancelAttempt = cancelAttempt
	return func() tea.Msg {
		defer cancelAttempt()
		if refresh == nil {
			return refreshFinishedMsg(RefreshResult{Generation: generation, Manual: manual, Err: errors.New("refresh is not configured")})
		}
		return refreshFinishedMsg(refresh(ctx, request))
	}, true
}

func (m Model) applyRefresh(result RefreshResult) Model {
	if result.Generation != m.generation || (result.Manual && result.Manual != (m.manual > 0)) {
		return m
	}
	if m.cancelAttempt != nil {
		m.cancelAttempt()
		m.cancelAttempt = nil
	}
	m.inFlight = false
	if result.NextAllowed.After(m.nextAllowed) {
		m.nextAllowed = result.NextAllowed
	}
	if result.AuthFailure {
		m.state, m.suspended, m.manualSuccess = ViewSetup, true, false
		m.notice = "Kimi rejected the token. Run kimiburn setup; automatic polling is suspended."
		if result.Err != nil {
			m.errText = result.Err.Error()
		}
		return m
	}
	if result.Err != nil {
		if m.snapshot.GeneratedAt.IsZero() {
			m.state, m.errText = ViewFatal, result.Err.Error()
		} else {
			m.state, m.warning = ViewReady, result.Err.Error()
		}
		return m
	}
	m.snapshot, m.cached, m.state, m.warning = result.Snapshot, false, ViewReady, ""
	if result.History != nil {
		m.history = result.History
	}
	if result.Manual {
		m.suspended, m.manualSuccess = false, true
	}
	return m
}

func (m Model) applyCache(result CacheResult) Model {
	if result.Err == nil {
		m.snapshot, m.cached, m.state, m.warning = result.Snapshot, true, ViewReady, ""
		return m
	}
	if m.snapshot.GeneratedAt.IsZero() {
		m.state, m.errText = ViewFatal, result.Err.Error()
		m.notice = "No cached snapshot is available."
	} else {
		m.state, m.warning = ViewReady, result.Err.Error()
	}
	return m
}

func (m Model) scheduleTick(now time.Time) tea.Cmd {
	delay, _ := m.ScheduleDelay(now, m.interval)
	return func() tea.Msg { time.Sleep(maxDuration(0, delay)); return tickMsg{} }
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
