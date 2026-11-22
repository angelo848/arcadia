package input

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// ErrDisconnected is wrapped by PadAPI.State when the controller is not connected.
var ErrDisconnected = errors.New("input: controller disconnected")

// XInput button bits.
const (
	ButtonDPadUp   uint16 = 0x0001
	ButtonDPadDown uint16 = 0x0002
	ButtonBack     uint16 = 0x0020
	ButtonA        uint16 = 0x1000
	ButtonB        uint16 = 0x2000
	ButtonY        uint16 = 0x8000
)

// PadState is a snapshot of one controller.
type PadState struct{ Buttons uint16 }

// PadAPI reads controller state; errors wrapping ErrDisconnected mean "not connected".
type PadAPI interface {
	State(slot int) (PadState, error)
}

// PadConfig tunes the poller; zero fields take the defaults.
type PadConfig struct {
	Hold, Poll, Rescan time.Duration
	Mask               uint16
	CloseMask          uint16 // default Back+B
	BigBoxMask         uint16 // default Back+A
	// Volume is called with true for Back+D-pad up and false for Back+D-pad down, on the poller goroutine,
	// at the first press and then repeated while held. Nil disables the volume chords.
	Volume                   func(up bool)
	RepeatDelay, RepeatEvery time.Duration
}

type pad struct {
	api    PadAPI
	cfg    PadConfig
	clock  domain.Clock
	log    *slog.Logger
	det    comboDetector
	cdet   comboDetector
	bdet   comboDetector
	vup    repeatDetector
	vdown  repeatDetector
	mu     sync.Mutex
	stop   chan struct{}
	wg     sync.WaitGroup
	closed bool
	// failing marks slots whose last read failed with a non-disconnect error;
	// touched only by Start and then the loop goroutine.
	failing [padSlots]bool
}

// NewPad returns a gamepad Source polling api through clock.
func NewPad(api PadAPI, cfg PadConfig, clock domain.Clock, log *slog.Logger) Source {
	if cfg.Hold <= 0 {
		cfg.Hold = defaultHold
	}
	if cfg.Poll <= 0 {
		cfg.Poll = defaultPoll
	}
	if cfg.Rescan <= 0 {
		cfg.Rescan = defaultRescan
	}
	if cfg.Mask == 0 {
		cfg.Mask = defaultMask
	}
	if cfg.CloseMask == 0 {
		cfg.CloseMask = defaultCloseMask
	}
	if cfg.BigBoxMask == 0 {
		cfg.BigBoxMask = defaultBigBoxMask
	}
	if log == nil {
		log = slog.Default()
	}
	return &pad{
		api: api, cfg: cfg, clock: clock, log: log,
		det:   comboDetector{Hold: cfg.Hold, Mask: cfg.Mask},
		cdet:  comboDetector{Hold: cfg.Hold, Mask: cfg.CloseMask},
		bdet:  comboDetector{Hold: cfg.Hold, Mask: cfg.BigBoxMask},
		vup:   repeatDetector{Mask: ButtonBack | ButtonDPadUp, Delay: cfg.RepeatDelay, Every: cfg.RepeatEvery},
		vdown: repeatDetector{Mask: ButtonBack | ButtonDPadDown, Delay: cfg.RepeatDelay, Every: cfg.RepeatEvery},
	}
}

func (p *pad) Name() string { return "gamepad" }

func (p *pad) Start(out chan<- Trigger) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		return errors.New("input: gamepad already started")
	}
	p.stop = make(chan struct{})

	var connected [padSlots]bool
	var lastProbe [padSlots]time.Time
	now := p.clock.Now()
	for slot := 0; slot < padSlots; slot++ {
		p.read(slot, now, &connected, &lastProbe, out)
	}

	p.wg.Add(1)
	go p.loop(out, connected, lastProbe)
	return nil
}

func (p *pad) loop(out chan<- Trigger, connected [padSlots]bool, lastProbe [padSlots]time.Time) {
	defer p.wg.Done()
	for {
		select {
		case <-p.stop:
			return
		case now := <-p.clock.After(p.cfg.Poll):
			for slot := 0; slot < padSlots; slot++ {
				if !connected[slot] && now.Sub(lastProbe[slot]) < p.cfg.Rescan {
					continue
				}
				p.read(slot, now, &connected, &lastProbe, out)
			}
		}
	}
}

func (p *pad) read(slot int, now time.Time, connected *[padSlots]bool, lastProbe *[padSlots]time.Time, out chan<- Trigger) {
	lastProbe[slot] = now
	st, err := p.api.State(slot)
	if err != nil {
		// Log only the first failure of a sequence, not on every rescan.
		if !errors.Is(err, ErrDisconnected) && !p.failing[slot] {
			p.log.Warn("gamepad read failed", "slot", slot, "err", err)
		}
		p.failing[slot] = !errors.Is(err, ErrDisconnected)
		connected[slot] = false
		p.det.Observe(slot, 0, false, now)
		p.cdet.Observe(slot, 0, false, now)
		p.bdet.Observe(slot, 0, false, now)
		p.vup.Observe(slot, 0, false, now)
		p.vdown.Observe(slot, 0, false, now)
		return
	}
	p.failing[slot] = false
	connected[slot] = true
	if p.det.Observe(slot, st.Buttons, true, now) {
		p.emit(out, Trigger{Source: domain.PauseGamepad, At: now, Action: ActionPause}, slot)
	}
	if p.cdet.Observe(slot, st.Buttons, true, now) {
		p.emit(out, Trigger{Source: domain.PauseGamepad, At: now, Action: ActionClose}, slot)
	}
	if p.bdet.Observe(slot, st.Buttons, true, now) {
		p.emit(out, Trigger{Source: domain.PauseGamepad, At: now, Action: ActionBigBox}, slot)
	}
	// The detectors always observe, so a release resets them even when the volume is off.
	up := p.vup.Observe(slot, st.Buttons, true, now)
	down := p.vdown.Observe(slot, st.Buttons, true, now)
	if p.cfg.Volume != nil {
		if up {
			p.cfg.Volume(true)
		}
		if down {
			p.cfg.Volume(false)
		}
	}
}

func (p *pad) emit(out chan<- Trigger, t Trigger, slot int) {
	select {
	case out <- t:
	default:
		p.log.Warn("gamepad trigger dropped: output queue full", "slot", slot, "action", int(t.Action))
	}
}

func (p *pad) Close() error {
	p.mu.Lock()
	if p.stop != nil && !p.closed {
		p.closed = true
		close(p.stop)
	}
	p.mu.Unlock()
	p.wg.Wait()
	return nil
}
