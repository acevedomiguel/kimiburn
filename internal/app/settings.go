package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/acevedomiguel/burnblocks/kimi"
	"github.com/acevedomiguel/kimiburn/internal/ui"
)

// tokenUpdateTimeout bounds the single validation read a token replacement
// performs, matching setup's timeout so the two paths behave the same.
const tokenUpdateTimeout = 20 * time.Second

// maxTokenInputLength caps the hidden token buffer so a stray paste cannot grow
// it without bound.
const maxTokenInputLength = 4096

// DefaultIntervalOptions are the automatic-refresh intervals the Settings
// overlay offers, in ascending order. Zero (manual refresh only) stays
// available through --refresh and the configuration file, but is not a menu
// choice. The renderer owns the list; this is a re-export for the CLI wiring.
func DefaultIntervalOptions() []time.Duration {
	return ui.DefaultIntervalOptions()
}

// intervalIndexFor returns the option index equal to interval, or -1 when the
// running interval is a custom value that is not one of the options.
func intervalIndexFor(interval time.Duration, options []time.Duration) int {
	for i, option := range options {
		if option == interval {
			return i
		}
	}
	return -1
}

// nearestIntervalIndex is the option index closest to interval, used as the
// starting point when a custom interval is not itself a menu option.
func nearestIntervalIndex(interval time.Duration, options []time.Duration) int {
	best, bestDelta := 0, time.Duration(-1)
	for i, option := range options {
		delta := interval - option
		if delta < 0 {
			delta = -delta
		}
		if bestDelta < 0 || delta < bestDelta {
			best, bestDelta = i, delta
		}
	}
	return best
}

// openSettings shows the Settings overlay over the current screen, remembering
// where to return when it closes.
func (m Model) openSettings() Model {
	m.savedState, m.savedScroll = m.state, m.scroll
	m.state = ViewSettings
	m.settingsFocus = 0
	m.settingsIndex = intervalIndexFor(m.interval, m.intervalOptions)
	m.tokenEditing, m.tokenInput, m.tokenSaving = false, "", false
	m.settingsMsg, m.settingsErr = "", false
	return m
}

// tokenAvailable reports whether in-app token replacement can run at all.
func (m Model) tokenAvailable() bool {
	return !m.demo && !m.envToken && m.validateToken != nil && m.saveToken != nil
}

// settingsOptions builds the pure render description for the overlay.
func (m Model) settingsOptions() ui.SettingsOptions {
	return ui.SettingsOptions{
		Width: m.width, Height: m.height, Styles: m.styles,
		IntervalOptions: m.intervalOptions, IntervalIndex: m.settingsIndex, Interval: m.interval,
		IntervalPinned: m.intervalPinned, IntervalSaved: m.setInterval != nil,
		Focus:          m.settingsFocus,
		TokenAvailable: m.tokenAvailable(), EnvToken: m.envToken,
		Editing: m.tokenEditing, TokenLength: len([]rune(m.tokenInput)), Saving: m.tokenSaving,
		Message: m.settingsMsg, MessageIsError: m.settingsErr,
	}
}

// settingsBackground renders the screen the Settings overlay sits above. A
// missing-token screen keeps its own frame so the overlay does not sit on an
// empty dashboard.
func (m Model) settingsBackground() string {
	if m.savedState == ViewSetup {
		return renderSimpleFrame("KIMIBURN · Setup",
			"No token is configured.\n\nUse Update token below to add one, or run: kimiburn setup",
			"[esc] close", m.width, m.height, m.styles)
	}
	return ui.RenderDashboard(m.snapshot, m.dashboardOptions(m.savedScroll))
}

// handleSettingsKey processes a key while the Settings overlay is open. While a
// token is being typed all printable input goes to the hidden buffer.
func (m Model) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.tokenEditing {
		return m.handleTokenInput(msg)
	}
	switch {
	case key.Matches(msg, m.settingsKeys.Quit):
		return m.quit()
	case key.Matches(msg, m.settingsKeys.Close):
		m.state, m.scroll = m.savedState, m.savedScroll
		return m, nil
	case key.Matches(msg, m.settingsKeys.Move):
		if msg.String() == "up" || msg.String() == "k" {
			m.settingsFocus = 0
		} else {
			m.settingsFocus = 1
		}
		return m, nil
	case key.Matches(msg, m.settingsKeys.Change):
		switch msg.String() {
		case "left", "h":
			return m.changeInterval(-1)
		}
		return m.changeInterval(1)
	case msg.Code >= '1' && msg.Code <= '9':
		return m.chooseInterval(int(msg.Code - '1'))
	case key.Matches(msg, m.settingsKeys.Edit):
		if m.settingsFocus == 1 && m.tokenAvailable() {
			m.tokenEditing, m.tokenInput = true, ""
			m.settingsMsg, m.settingsErr = "", false
		}
		return m, nil
	}
	return m, nil
}

// handleTokenInput collects the hidden token, then validates and saves it on
// Enter. Esc abandons the entry; every other printable key is token content, so
// the overlay's own letters (s, q) are not treated as commands here.
func (m Model) handleTokenInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.settingsKeys.Quit):
		return m.quit()
	case msg.Code == tea.KeyEscape:
		m.tokenEditing, m.tokenInput = false, ""
		m.settingsMsg, m.settingsErr = "", false
		return m, nil
	case msg.Code == tea.KeyEnter:
		return m.submitToken()
	case msg.Code == tea.KeyBackspace:
		if runes := []rune(m.tokenInput); len(runes) > 0 {
			m.tokenInput = string(runes[:len(runes)-1])
		}
		return m, nil
	case msg.Text != "" && len(m.tokenInput)+len(msg.Text) <= maxTokenInputLength:
		m.tokenInput += msg.Text
		return m, nil
	}
	return m, nil
}

// changeInterval moves the interval selection by delta and applies it. The
// interval row is the only one with a left/right control, so it responds even
// while the token row holds focus.
func (m Model) changeInterval(delta int) (tea.Model, tea.Cmd) {
	if len(m.intervalOptions) == 0 {
		return m, nil
	}
	idx := m.settingsIndex
	if idx < 0 {
		idx = nearestIntervalIndex(m.interval, m.intervalOptions)
	} else {
		idx += delta
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.intervalOptions) {
		idx = len(m.intervalOptions) - 1
	}
	return m.chooseInterval(idx)
}

// chooseInterval applies the option at idx: the session interval changes
// immediately and, when a persister is wired, the choice is saved.
func (m Model) chooseInterval(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.intervalOptions) {
		return m, nil
	}
	chosen := m.intervalOptions[idx]
	m.settingsIndex = idx
	m.interval = chosen

	var err error
	if m.setInterval != nil {
		err = m.setInterval(chosen)
	}
	switch {
	case err != nil:
		m.settingsMsg = "Applied for this session; could not save: " + kimi.Redact(err.Error())
		m.settingsErr = true
	case m.intervalPinned:
		m.settingsMsg = "Applied. The external override still wins on restart."
		m.settingsErr = false
	case m.setInterval == nil:
		m.settingsMsg = "Applied for this session."
		m.settingsErr = false
	default:
		m.settingsMsg = "Saved to the configuration file."
		m.settingsErr = false
	}
	return m, m.armTick()
}

// submitToken starts the asynchronous validate-then-save for the entered token.
// The buffer is cleared immediately so the secret is not retained in the model.
func (m Model) submitToken() (tea.Model, tea.Cmd) {
	token := strings.TrimSpace(m.tokenInput)
	m.tokenInput = ""
	if token == "" {
		m.settingsMsg, m.settingsErr = "Type a token, then press Enter.", true
		return m, nil
	}
	if !m.tokenAvailable() {
		m.settingsMsg, m.settingsErr = "Token updates are unavailable in this mode.", true
		return m, nil
	}
	m.tokenEditing, m.tokenSaving = false, true
	m.settingsMsg, m.settingsErr = "Checking the token…", false

	validate, save := m.validateToken, m.saveToken
	redactor := kimi.NewRedactor(token)
	return m, func() tea.Msg {
		validateCtx, cancelValidate := context.WithTimeout(context.Background(), tokenUpdateTimeout)
		err := validate(validateCtx, token)
		cancelValidate()
		if err != nil {
			return tokenSavedMsg{err: errors.New(redactor.Redact(err.Error()))}
		}
		saveCtx, cancelSave := context.WithTimeout(context.Background(), tokenUpdateTimeout)
		defer cancelSave()
		if err := save(saveCtx, token); err != nil {
			return tokenSavedMsg{err: errors.New(redactor.Redact(err.Error()))}
		}
		return tokenSavedMsg{}
	}
}

// applyTokenSaved reacts to a replacement token: a failure keeps the previous
// token and reports a redacted reason; a success clears any auth suspension and
// forces an immediate read with the new token.
func (m Model) applyTokenSaved(msg tokenSavedMsg) (Model, tea.Cmd) {
	m.tokenSaving = false
	if msg.err != nil {
		m.settingsMsg = "Not saved: " + msg.err.Error()
		m.settingsErr = true
		return m, nil
	}
	m.settingsMsg, m.settingsErr = "Token saved. Refreshing with the new token…", false

	// A missing-token screen has served its purpose once a token is stored, so
	// closing the overlay returns to the dashboard rather than back to setup.
	if m.savedState == ViewSetup || m.savedState == ViewFatal {
		m.savedState = ViewReady
	}
	m.suspended, m.manualSuccess = false, false

	// Supersede any in-flight request made with the previous token and clear the
	// cadence so the new credential is exercised right away. A provider that
	// still wants a slower pace re-imposes it through the next result's
	// NextAllowed, so the backoff is re-armed from the provider's own answer.
	m.generation++
	if m.cancelAttempt != nil {
		m.cancelAttempt()
		m.cancelAttempt = nil
	}
	m.inFlight = false
	m.lastAttempt = time.Time{}
	m.nextAllowed = time.Time{}
	return m, tea.Batch(m.startAttempt(true), m.armTick())
}
