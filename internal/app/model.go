package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/acevedomiguel/kimiburn/internal/domain"
	"github.com/acevedomiguel/kimiburn/internal/ui"
)

type viewState int

const (
	ViewLoading viewState = iota
	ViewReady
	ViewSetup
	ViewFatal
	ViewHelp
)

// RefreshRequest identifies one attempt. Generation prevents stale results;
// Manual distinguishes operator refreshes from the suspended scheduler.
type RefreshRequest struct {
	Generation uint64
	Manual     bool
}

// RefreshResult contains only provider-neutral data and sanitized errors.
type RefreshResult struct {
	Generation  uint64
	Manual      bool
	Snapshot    domain.Snapshot
	Err         error
	AuthFailure bool
	NextAllowed time.Time
	// History is a copy of the retained local samples after this reading; nil
	// leaves the displayed history unchanged.
	History map[domain.QuotaKey][]domain.Sample
}

type RefreshFunc func(context.Context, RefreshRequest) RefreshResult

type CacheResult struct {
	Snapshot domain.Snapshot
	Err      error
}

type initMsg struct{}
type tickMsg struct{}
type clockMsg struct{}
type cacheLoadedMsg CacheResult
type refreshFinishedMsg RefreshResult

// Options supplies all side effects and clock inputs to the interactive model.
type Options struct {
	Now         func() time.Time
	Cancel      context.CancelFunc
	Configured  func() (bool, error)
	Refresh     RefreshFunc
	Demo        bool
	Snapshot    domain.Snapshot
	History     map[domain.QuotaKey][]domain.Sample
	Width       int
	Height      int
	NoColor     bool
	Interval    time.Duration
	IntervalSet bool
	Warning     string
	// Theme is the configured palette: "dark" or "light" pin it, and "auto"
	// or empty follows the terminal's reported background colour.
	Theme string
	// TerminalProgress mirrors the 5-hour quota into the terminal's own
	// progress indicator (OSC 9;4), for terminals known to support it.
	TerminalProgress bool
}

// Model is the complete Bubble Tea state for one Kimiburn process.
type Model struct {
	state         viewState
	snapshot      domain.Snapshot
	history       map[domain.QuotaKey][]domain.Sample
	cached        bool
	width         int
	height        int
	noColor       bool
	theme         string
	styles        ui.Styles
	keys          ui.KeyMap
	helpKeys      ui.HelpKeyMap
	spinner       spinner.Model
	meters        map[domain.QuotaKey]meter
	meterGen      uint64
	scroll        int
	savedState    viewState
	savedScroll   int
	interval      time.Duration
	now           func() time.Time
	cancel        context.CancelFunc
	cancelAttempt context.CancelFunc
	configured    func() (bool, error)
	refresh       RefreshFunc
	demo          bool
	termProgress  bool
	started       bool
	inFlight      bool
	generation    uint64
	manual        uint64
	manualSuccess bool
	suspended     bool
	lastAttempt   time.Time
	nextAllowed   time.Time
	nextTick      time.Time
	warning       string
	notice        string
	errText       string
}

func NewModel(opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Width <= 0 {
		opts.Width = 80
	}
	if opts.Height <= 0 {
		opts.Height = 24
	}
	if !opts.IntervalSet && opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}
	if opts.Demo {
		// The demo clock starts at the fixture's reading time and advances in
		// real time, so its countdowns tick down from the README values.
		wall := opts.Now
		start := wall()
		opts.Now = func() time.Time { return ui.SyntheticTime().Add(wall().Sub(start)) }
		if opts.History == nil {
			opts.History = ui.SyntheticHistory()
		}
	}
	if opts.Demo && opts.Refresh == nil {
		opts.Refresh = func(_ context.Context, request RefreshRequest) RefreshResult {
			// The demo renders the same fixture the README screenshot shows, so
			// `kimiburn demo` reproduces that image.
			return RefreshResult{Generation: request.Generation, Manual: request.Manual, Snapshot: ui.SyntheticLegacySnapshot(), History: ui.SyntheticHistory()}
		}
	}
	state := ViewLoading
	if opts.Snapshot.GeneratedAt.IsZero() == false {
		state = ViewReady
	}
	styles := ui.NewStylesFor(!opts.NoColor, opts.Theme != "light")
	return Model{
		state: state, snapshot: opts.Snapshot, history: opts.History, width: opts.Width, height: opts.Height,
		noColor: opts.NoColor, theme: opts.Theme, styles: styles, keys: ui.NewKeyMap(), helpKeys: ui.NewHelpKeyMap(),
		spinner: ui.NewSpinner(styles), interval: opts.Interval, now: opts.Now,
		cancel: opts.Cancel, configured: opts.Configured, refresh: opts.Refresh,
		demo: opts.Demo, termProgress: opts.TerminalProgress, warning: opts.Warning,
	}
}

func (m Model) Init() tea.Cmd { return func() tea.Msg { return initMsg{} } }

// clockTick re-renders once a second so reset countdowns and the next-refresh
// caption stay current between readings. It performs no I/O.
func clockTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockMsg{} })
}

// stateInit selects the first screen without performing I/O itself.
func stateInit(m Model, _ time.Time) Model {
	if m.demo || m.snapshot.GeneratedAt.IsZero() == false {
		return m
	}
	if m.configured == nil {
		m.state = ViewSetup
		return m
	}
	configured, err := m.configured()
	if err != nil {
		m.state, m.errText = ViewFatal, err.Error()
		return m
	}
	if !configured {
		m.state = ViewSetup
	}
	return m
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initMsg:
		m = stateInit(m, m.now())
		if m.state == ViewSetup || m.state == ViewFatal {
			return m, m.startup()
		}
		// The first read is returned on its own; the rest of the startup work
		// follows its result, so the first message is always the reading.
		cmd, _ := m.StartAttempt(false)
		return m, cmd
	case tea.BackgroundColorMsg:
		if m.theme == "dark" || m.theme == "light" {
			return m, nil
		}
		m.styles = ui.NewStylesFor(!m.noColor, msg.IsDark())
		m.spinner.Style = ui.NewSpinner(m.styles).Style
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll = minInt(maxInt(0, m.scroll), m.maxScroll())
		return m, nil
	case refreshFinishedMsg:
		m = m.applyRefresh(RefreshResult(msg))
		startup := m.startup()
		meters := m.syncMeters()
		tick := m.armTick()
		return m, tea.Batch(startup, meters, tick)
	case cacheLoadedMsg:
		m = m.applyCache(CacheResult(msg))
		startup := m.startup()
		meters := m.syncMeters()
		tick := m.armTick()
		return m, tea.Batch(startup, meters, tick)
	case tickMsg:
		attempt := m.startAttempt(false)
		tick := m.armTick()
		return m, tea.Batch(attempt, tick)
	case clockMsg:
		return m, clockTick()
	case meterFrameMsg:
		cmd := m.stepMeters(msg)
		return m, cmd
	case spinner.TickMsg:
		if !m.inFlight {
			// Stop animating once nothing is in flight.
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollBy(-3)
		case tea.MouseWheelDown:
			m.scrollBy(3)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// startup runs once: it asks the terminal for its background colour, so the
// palette suits light and dark themes, and starts the one-second clock that
// keeps countdowns live. Neither performs provider I/O.
func (m *Model) startup() tea.Cmd {
	if m.started {
		return nil
	}
	m.started = true
	return tea.Batch(tea.RequestBackgroundColor, clockTick())
}

// startAttempt wraps StartAttempt with the spinner that marks it in flight.
func (m *Model) startAttempt(manual bool) tea.Cmd {
	cmd, ok := m.StartAttempt(manual)
	if !ok {
		return nil
	}
	return tea.Batch(cmd, m.spinner.Tick)
}

// armTick schedules the next automatic read and remembers when it is due, for
// the countdown in the freshness caption.
func (m *Model) armTick() tea.Cmd {
	now := m.now()
	delay, _ := m.ScheduleDelay(now, m.interval)
	m.nextTick = now.Add(delay)
	return m.scheduleTick(now)
}

func (m *Model) scrollBy(delta int) {
	switch m.state {
	case ViewReady:
		m.scroll = minInt(maxInt(0, m.scroll+delta), m.maxScroll())
	case ViewHelp:
		m.scroll = minInt(maxInt(0, m.scroll+delta), m.maxHelpScroll())
	}
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	if m.cancelAttempt != nil {
		m.cancelAttempt()
	}
	return m, tea.Quit
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := maxInt(1, m.height-3)
	if m.state == ViewHelp {
		// Esc, ? and q close the overlay and return to the dashboard; only
		// ctrl+c quits from here.
		switch {
		case key.Matches(msg, m.helpKeys.Close):
			m.state, m.scroll = m.savedState, m.savedScroll
		case key.Matches(msg, m.helpKeys.Quit):
			return m.quit()
		case key.Matches(msg, m.keys.Up):
			m.scrollBy(-1)
		case key.Matches(msg, m.keys.Down):
			m.scrollBy(1)
		case key.Matches(msg, m.keys.PageUp):
			m.scrollBy(-page)
		case key.Matches(msg, m.keys.PageDown):
			m.scrollBy(page)
		case key.Matches(msg, m.keys.Home):
			m.scroll = 0
		case key.Matches(msg, m.keys.End):
			m.scroll = m.maxHelpScroll()
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Escape):
		return m.quit()
	case key.Matches(msg, m.keys.Refresh):
		cmd := m.startAttempt(true)
		return m, cmd
	case key.Matches(msg, m.keys.Help):
		if m.state == ViewReady {
			m = m.openHelp()
		}
	case key.Matches(msg, m.keys.Up):
		m.scrollBy(-1)
	case key.Matches(msg, m.keys.Down):
		m.scrollBy(1)
	case key.Matches(msg, m.keys.PageUp):
		m.scrollBy(-page)
	case key.Matches(msg, m.keys.PageDown):
		m.scrollBy(page)
	case key.Matches(msg, m.keys.Home):
		if m.state == ViewReady {
			m.scroll = 0
		}
	case key.Matches(msg, m.keys.End):
		if m.state == ViewReady {
			m.scroll = m.maxScroll()
		}
	}
	return m, nil
}

// dashboardOptions is the single description of the dashboard frame, shared by
// rendering and by scroll-limit measurement.
func (m Model) dashboardOptions(scroll int) ui.DashboardOptions {
	next := time.Time{}
	if !m.suspended && m.interval > 0 {
		next = m.nextTick
	}
	return ui.DashboardOptions{
		Width: m.width, Height: m.height, Now: m.now(), Cached: m.cached, Demo: m.demo,
		Styles: m.styles, Warning: m.warning, Scroll: scroll,
		History: m.history, Meters: m.shownMeters(),
		Refreshing: m.inFlight, Spinner: m.spinner.View(), NextRefresh: next,
	}
}

func (m Model) View() tea.View {
	var content string
	switch m.state {
	case ViewReady:
		content = ui.RenderDashboard(m.snapshot, m.dashboardOptions(m.scroll))
	case ViewHelp:
		opts := m.dashboardOptions(m.savedScroll)
		opts.HelpOpen = true
		background := ui.RenderDashboard(m.snapshot, opts)
		content = ui.HelpOverlay(background, m.styles, m.width, m.height, m.scroll, m.warnings())
	case ViewSetup:
		body := "Token setup required\n\nRun: kimiburn setup\n\nThe interactive app never reads a token from the terminal."
		if m.notice != "" {
			body += "\n\n" + m.notice
		}
		content = renderSimpleFrame("KIMIBURN · Setup", body, "[q] quit", m.width, m.height, m.styles)
	case ViewFatal:
		body := "Refresh failed\n\n" + m.errText
		content = renderSimpleFrame("KIMIBURN · Error", body, "[q] quit", m.width, m.height, m.styles)
	default:
		loading := "Loading latest Kimi Code quota…"
		if m.inFlight {
			loading = m.spinner.View() + " " + loading
		}
		content = renderSimpleFrame("KIMIBURN", loading, "[q] quit", m.width, m.height, m.styles)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = m.windowTitle()
	if m.termProgress {
		view.ProgressBar = m.terminalProgress()
	}
	return view
}

// fiveHour returns the 5-hour quota's remaining fraction when it is valid.
func (m Model) fiveHour() (float64, bool) {
	if m.state != ViewReady && m.state != ViewHelp {
		return 0, false
	}
	quota, ok := m.snapshot.Quotas[domain.QuotaLimit5h]
	if !ok || quota.UsedRatio == nil || *quota.UsedRatio < 0 || *quota.UsedRatio > 1 {
		return 0, false
	}
	return 1 - *quota.UsedRatio, true
}

// windowTitle puts the most time-critical figures in the terminal tab.
func (m Model) windowTitle() string {
	remaining, ok := m.fiveHour()
	if !ok {
		return "kimiburn"
	}
	title := "kimiburn · 5h " + ui.FormatPercent(&remaining) + " left"
	if runway := m.snapshot.Runways[domain.QuotaLimit5h]; runway.Status == "finite" {
		title += " · runway " + ui.FormatRunway(runway, domain.QuotaLimit5h)
	}
	return title
}

// terminalProgress mirrors the 5-hour quota into the terminal's progress
// indicator: a warning when it runs out before reset or has under a fifth
// left, an error when depleted.
func (m Model) terminalProgress() *tea.ProgressBar {
	remaining, ok := m.fiveHour()
	if !ok {
		return tea.NewProgressBar(tea.ProgressBarNone, 0)
	}
	state := tea.ProgressBarDefault
	switch status := m.snapshot.Runways[domain.QuotaLimit5h].Status; {
	case status == "depleted" || remaining <= 0:
		state = tea.ProgressBarError
	case status == "finite" || remaining < 0.2:
		state = tea.ProgressBarWarning
	}
	return tea.NewProgressBar(state, int(remaining*100+0.5))
}

func (m Model) openHelp() Model {
	m.savedState, m.savedScroll = m.state, m.scroll
	m.state, m.scroll = ViewHelp, 0
	return m
}

// warnings lists every current warning for the Help overlay.
func (m Model) warnings() []string {
	var out []string
	if m.warning != "" {
		out = append(out, m.warning)
	}
	for _, warning := range m.snapshot.Warnings {
		out = append(out, warning.Section+": "+warning.Message)
	}
	if m.notice != "" {
		out = append(out, m.notice)
	}
	return out
}

// helpLines is the Help document as the overlay lays it out.
func (m Model) helpLines() []string {
	return ui.HelpContent(m.styles, ui.HelpTextWidth(m.width), m.warnings())
}

func (m Model) maxScroll() int {
	if m.state != ViewReady {
		return 0
	}
	body := ui.BuildDashboardBody(m.snapshot, m.dashboardOptions(0))
	bodyHeight := ui.BodyHeight(m.height, ui.DashboardWarning(m.snapshot, m.warning) != "")
	return maxInt(0, len(body)-bodyHeight)
}

func (m Model) maxHelpScroll() int {
	return ui.HelpScrollLimit(m.styles, m.width, m.height, m.warnings())
}

func renderSimpleFrame(title, body, footer string, width, height int, styles ui.Styles) string {
	width, height = maxInt(1, width), maxInt(1, height)
	if height < 3 {
		return fitLine(body, width)
	}
	wrapped := wrapPlainText(body, maxInt(1, width-4))
	out := make([]string, 0, height)
	out = append(out, fitLine(styles.Title.Render(title), width))
	for len(out) < height-1-len(wrapped) {
		out = append(out, strings.Repeat(" ", width))
	}
	for _, line := range wrapped {
		if len(out) >= height-1 {
			break
		}
		out = append(out, fitLine("  "+line, width))
	}
	for len(out) < height-1 {
		out = append(out, strings.Repeat(" ", width))
	}
	out = append(out, fitLine(footer, width))
	return strings.Join(out, "\n")
}

func fitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "…")
	return line + strings.Repeat(" ", maxInt(0, width-lipgloss.Width(line)))
}

func wrapPlainText(text string, width int) []string {
	if width < 1 {
		return []string{""}
	}
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
