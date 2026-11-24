package app

import (
	"context"
	"strings"
	"time"

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
}

type RefreshFunc func(context.Context, RefreshRequest) RefreshResult

type CacheResult struct {
	Snapshot domain.Snapshot
	Err      error
}

type initMsg struct{}
type tickMsg struct{}
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
	Width       int
	Height      int
	NoColor     bool
	Interval    time.Duration
	IntervalSet bool
	Warning     string
}

// Model is the complete Bubble Tea state for one Kimiburn process.
type Model struct {
	state         viewState
	snapshot      domain.Snapshot
	cached        bool
	width         int
	height        int
	styles        ui.Styles
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
	inFlight      bool
	generation    uint64
	manual        uint64
	manualSuccess bool
	suspended     bool
	lastAttempt   time.Time
	nextAllowed   time.Time
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
	if opts.Demo && opts.Refresh == nil {
		opts.Refresh = func(_ context.Context, request RefreshRequest) RefreshResult {
			return RefreshResult{Generation: request.Generation, Manual: request.Manual, Snapshot: ui.SyntheticSnapshot()}
		}
	}
	state := ViewLoading
	if opts.Snapshot.GeneratedAt.IsZero() == false {
		state = ViewReady
	}
	return Model{
		state: state, snapshot: opts.Snapshot, width: opts.Width, height: opts.Height,
		styles: ui.NewStyles(!opts.NoColor), interval: opts.Interval, now: opts.Now,
		cancel: opts.Cancel, configured: opts.Configured, refresh: opts.Refresh,
		demo: opts.Demo, warning: opts.Warning,
	}
}

func (m Model) Init() tea.Cmd { return func() tea.Msg { return initMsg{} } }

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
			return m, nil
		}
		cmd, _ := m.StartAttempt(false)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll = minInt(maxInt(0, m.scroll), m.maxScroll())
		return m, nil
	case refreshFinishedMsg:
		result := RefreshResult(msg)
		m = m.applyRefresh(result)
		return m, m.scheduleTick(m.now())
	case cacheLoadedMsg:
		m = m.applyCache(CacheResult(msg))
		return m, m.scheduleTick(m.now())
	case tickMsg:
		cmd, _ := m.StartAttempt(false)
		return m, tea.Batch(cmd, m.scheduleTick(m.now()))
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		if m.cancel != nil {
			m.cancel()
		}
		if m.cancelAttempt != nil {
			m.cancelAttempt()
		}
		return m, tea.Quit
	}
	if m.state == ViewHelp {
		switch key {
		case "q", "?":
			m.state, m.scroll = m.savedState, m.savedScroll
		case "up", "k":
			m.scroll = maxInt(0, m.scroll-1)
		case "down", "j":
			m.scroll = minInt(m.scroll+1, m.maxHelpScroll())
		case "pgup":
			m.scroll = maxInt(0, m.scroll-(m.height-3))
		case "pgdown":
			m.scroll = minInt(m.scroll+(m.height-3), m.maxHelpScroll())
		case "home":
			m.scroll = 0
		case "end":
			m.scroll = m.maxHelpScroll()
		}
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		if m.cancelAttempt != nil {
			m.cancelAttempt()
		}
		return m, tea.Quit
	case "r":
		cmd, _ := m.StartAttempt(true)
		return m, cmd
	case "?":
		if m.state == ViewReady {
			m = m.openHelp()
		}
	case "up", "k":
		if m.state == ViewReady {
			m.scroll = maxInt(0, m.scroll-1)
		}
	case "down", "j":
		if m.state == ViewReady {
			m.scroll = minInt(m.scroll+1, m.maxScroll())
		}
	case "pgup":
		if m.state == ViewReady {
			m.scroll = maxInt(0, m.scroll-(m.height-3))
		}
	case "pgdown":
		if m.state == ViewReady {
			m.scroll = minInt(m.scroll+(m.height-3), m.maxScroll())
		}
	case "home":
		if m.state == ViewReady {
			m.scroll = 0
		}
	case "end":
		if m.state == ViewReady {
			m.scroll = m.maxScroll()
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch m.state {
	case ViewReady:
		content = ui.RenderDashboard(m.snapshot, ui.DashboardOptions{Width: m.width, Height: m.height, Now: m.now(), Cached: m.cached, Demo: m.demo, Styles: m.styles, Warning: m.warning, Scroll: m.scroll})
	case ViewHelp:
		content = m.renderHelp()
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
		content = renderSimpleFrame("KIMIBURN", "Loading latest Kimi Code quota…", "[q] quit", m.width, m.height, m.styles)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m Model) openHelp() Model {
	m.savedState, m.savedScroll = m.state, m.scroll
	m.state, m.scroll = ViewHelp, 0
	return m
}

func (m Model) helpLines() []string {
	lines := []string{
		"KIMIBURN · Help",
		"",
		"Monthly membership is the shared membership quota. Monthly Code allocation",
		"is included in that membership, not an additional allowance.",
		"Runway is estimated locally from retained readings and bounded by reset time.",
		"It is approximate, not a server-provided prediction.",
		"Finite forecasts stop strictly before reset; Beyond reset has no timestamp.",
		"No consumption means burn was zero/negative; Depleted means 100% used.",
		"Unavailable · insufficient samples needs 5 samples over at least 120 seconds.",
		"A reset or ratio decrease clears only that quota's local history.",
		"",
		"[r] refresh   [↑↓/j/k] scroll   [pgup/pgdn] page   [home/end] jump",
		"[?] help   [q] quit",
		"",
		"Warnings:",
	}
	if m.warning != "" {
		lines = append(lines, wrapPlainText(m.warning, maxInt(20, m.width-2))...)
	}
	for _, warning := range m.snapshot.Warnings {
		lines = append(lines, wrapPlainText(warning.Section+": "+warning.Message, maxInt(20, m.width-2))...)
	}
	if m.notice != "" {
		lines = append(lines, wrapPlainText(m.notice, maxInt(20, m.width-2))...)
	}
	if m.warning == "" && len(m.snapshot.Warnings) == 0 && m.notice == "" {
		lines = append(lines, "None")
	}
	return lines
}

func (m Model) renderHelp() string {
	lines := m.helpLines()
	bodyHeight := maxInt(1, m.height-2)
	scroll := minInt(m.scroll, maxInt(0, len(lines)-bodyHeight))
	end := minInt(len(lines), scroll+bodyHeight)
	out := make([]string, 0, m.height)
	out = append(out, fitLine(m.styles.Title.Render("KIMIBURN · Details"), m.width))
	for _, line := range lines[scroll:end] {
		out = append(out, fitLine(line, m.width))
	}
	for len(out) < m.height-1 {
		out = append(out, strings.Repeat(" ", m.width))
	}
	out = append(out, fitLine("[↑↓/j/k] scroll · [?/q] close · [esc] quit", m.width))
	return strings.Join(out[:m.height], "\n")
}

func (m Model) maxScroll() int {
	if m.state != ViewReady {
		return 0
	}
	chrome := 2
	if m.warning != "" || len(m.snapshot.Warnings) > 0 {
		chrome++
	}
	bodyHeight := maxInt(1, m.height-chrome)
	return maxInt(0, len(ui.BuildDashboardBody(m.snapshot, ui.DashboardOptions{Width: m.width, Height: m.height, Now: m.now(), Cached: m.cached, Styles: m.styles}))-bodyHeight)
}

func (m Model) maxHelpScroll() int { return maxInt(0, len(m.helpLines())-(m.height-2)) }

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
