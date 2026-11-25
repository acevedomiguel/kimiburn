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
	if m.snapshot.GeneratedAt.IsZero() && m.state != ViewSettings && m.state != ViewHelp {
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
	// An open Help or Settings overlay stays open across a background result;
	// only the data behind it changes. That keeps the scheduler invisible while
	// the operator is reading or editing settings.
	overlay := m.state == ViewHelp || m.state == ViewSettings
	if result.AuthFailure {
		m.suspended, m.manualSuccess = true, false
		m.notice = "Kimi rejected the token. Run kimiburn setup; automatic polling is suspended."
		if result.Err != nil {
			m.errText = result.Err.Error()
		}
		switch m.state {
		case ViewSettings:
			m.settingsMsg, m.settingsErr = "Kimi rejected the token. Automatic polling is suspended.", true
		case ViewHelp:
		default:
			m.state = ViewSetup
		}
		return m
	}
	if result.Err != nil {
		if m.snapshot.GeneratedAt.IsZero() {
			m.errText = result.Err.Error()
			if !overlay {
				m.state = ViewFatal
			}
		} else {
			m.warning = result.Err.Error()
			if !overlay {
				m.state = ViewReady
			}
		}
		return m
	}
	m.snapshot, m.cached, m.warning = result.Snapshot, false, ""
	if result.History != nil {
		m.history = result.History
	}
	if result.Manual {
		m.suspended, m.manualSuccess = false, true
	}
	if !overlay {
		m.state = ViewReady
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

// scheduleTick returns a timer that fires once after the current schedule
// delay. gen identifies the chain so a superseded timer can be ignored.
func (m Model) scheduleTick(now time.Time, gen uint64) tea.Cmd {
	delay, _ := m.ScheduleDelay(now, m.interval)
	return func() tea.Msg { time.Sleep(maxDuration(0, delay)); return tickMsg{gen: gen} }
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
