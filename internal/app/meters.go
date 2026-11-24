package app

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

// meterFPS is the animation rate of the quota meters.
const meterFPS = 60

// meterSpring is the spring the progress bubble uses, tuned a little stiffer
// so a refresh settles in well under a second without overshoot.
var meterSpring = harmonica.NewSpring(harmonica.FPS(meterFPS), 18, 1)

// meterOrder fixes the iteration order of the animated meters.
var meterOrder = []domain.QuotaKey{domain.QuotaLimit5h, domain.QuotaLimitMonthTotal, domain.QuotaLimitMonthCode, domain.QuotaLimit7d}

// meter is one animated remaining-quota bar: what it shows now, how fast it is
// moving, and the reading it is moving towards.
type meter struct {
	shown, velocity, target float64
}

func (m meter) settled() bool {
	return math.Abs(m.shown-m.target) < 0.001 && math.Abs(m.velocity) < 0.01
}

// meterFrameMsg advances every meter one animation step. gen drops frames from
// an animation a newer reading has already restarted.
type meterFrameMsg struct{ gen uint64 }

func meterFrame(gen uint64) tea.Cmd {
	return tea.Tick(time.Second/meterFPS, func(time.Time) tea.Msg { return meterFrameMsg{gen: gen} })
}

// syncMeters points each meter at the latest reading and starts the animation.
// A meter seen for the first time fills from empty, so the dashboard opens
// with the bars sweeping up to their values.
func (m *Model) syncMeters() tea.Cmd {
	m.meters = cloneMeters(m.meters)
	moving := false
	for _, key := range meterOrder {
		quota, ok := m.snapshot.Quotas[key]
		if !ok || quota.UsedRatio == nil || math.IsNaN(*quota.UsedRatio) || *quota.UsedRatio < 0 || *quota.UsedRatio > 1 {
			delete(m.meters, key)
			continue
		}
		current := m.meters[key]
		current.target = 1 - *quota.UsedRatio
		m.meters[key] = current
		moving = moving || !current.settled()
	}
	if !moving {
		return nil
	}
	m.meterGen++
	return meterFrame(m.meterGen)
}

// stepMeters advances the springs one frame and keeps ticking until all rest.
func (m *Model) stepMeters(msg meterFrameMsg) tea.Cmd {
	if msg.gen != m.meterGen {
		return nil
	}
	m.meters = cloneMeters(m.meters)
	moving := false
	for key, current := range m.meters {
		if current.settled() {
			current.shown, current.velocity = current.target, 0
		} else {
			current.shown, current.velocity = meterSpring.Update(current.shown, current.velocity, current.target)
			moving = true
		}
		m.meters[key] = current
	}
	if !moving {
		return nil
	}
	return meterFrame(m.meterGen)
}

// shownMeters is what each bar currently draws, for the renderer.
func (m Model) shownMeters() map[domain.QuotaKey]float64 {
	shown := make(map[domain.QuotaKey]float64, len(m.meters))
	for key, current := range m.meters {
		shown[key] = math.Max(0, math.Min(1, current.shown))
	}
	return shown
}

// cloneMeters copies the meter map before a change, so a Model value that was
// copied earlier never sees a later frame's state.
func cloneMeters(in map[domain.QuotaKey]meter) map[domain.QuotaKey]meter {
	out := make(map[domain.QuotaKey]meter, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
