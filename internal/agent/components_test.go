package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// --- fake components for testing ---

type fakeSource struct {
	mu        sync.Mutex
	name      string
	startErr  error
	started   bool
	closed    bool
	out       chan<- input.Trigger
	closeChan chan struct{}
}

func newFakeSource(name string, startErr error) *fakeSource {
	return &fakeSource{
		name:      name,
		startErr:  startErr,
		closeChan: make(chan struct{}),
	}
}

func (s *fakeSource) Name() string { return s.name }

func (s *fakeSource) Start(out chan<- input.Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startErr != nil {
		return s.startErr
	}
	s.started = true
	s.out = out
	return nil
}

func (s *fakeSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	select {
	case <-s.closeChan:
	default:
		close(s.closeChan)
	}
	return nil
}

func (s *fakeSource) emit(t input.Trigger) {
	s.mu.Lock()
	out := s.out
	s.mu.Unlock()
	if out != nil {
		out <- t
	}
}

type fakePowerWatcher struct {
	mu        sync.Mutex
	onSuspend func(at time.Time)
	onWake    func(at time.Time, userPresent bool)
	closed    bool
}

func (p *fakePowerWatcher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

type blockingOverlay struct {
	mu        sync.Mutex
	blockShow chan struct{}
	shows     []feedback.Notice
	closed    bool
}

func (o *blockingOverlay) Show(n feedback.Notice) {
	o.mu.Lock()
	o.shows = append(o.shows, n)
	block := o.blockShow
	o.mu.Unlock()
	if block != nil {
		<-block
	}
}

func (o *blockingOverlay) Hide() {}

func (o *blockingOverlay) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = true
	return nil
}

type fakeSoundRecorder struct {
	mu     sync.Mutex
	cues   []feedback.Cue
	closed bool
}

func (s *fakeSoundRecorder) Play(c feedback.Cue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cues = append(s.cues, c)
}

func (s *fakeSoundRecorder) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func TestFeedbackWiring(t *testing.T) {
	t.Run("FBK-002/feedback.Event e tracker.FeedbackKind têm os mesmos valores", func(t *testing.T) {
		if int(feedback.EventPaused) != int(tracker.FeedbackPaused) {
			t.Errorf("EventPaused (%d) != FeedbackPaused (%d)", feedback.EventPaused, tracker.FeedbackPaused)
		}
		if int(feedback.EventResumed) != int(tracker.FeedbackResumed) {
			t.Errorf("EventResumed (%d) != FeedbackResumed (%d)", feedback.EventResumed, tracker.FeedbackResumed)
		}
	})

	t.Run("FBK-003/Overlay.Show que bloqueia 5 s reais não atrasa as transições nem o Notify (tracker real com FakeClock)", func(t *testing.T) {
		blocker := make(chan struct{})
		defer func() {
			select {
			case <-blocker:
			default:
				close(blocker)
			}
		}()

		overlay := &blockingOverlay{blockShow: blocker}
		sound := &fakeSoundRecorder{}

		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						NewOverlay: func() (feedback.Overlay, error) { return overlay, nil },
						NewSound:   func() (feedback.Sound, error) { return sound, nil },
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Block Game")
		w.launch(t, c, gameID)

		// Pause the game via session.pause
		start := time.Now()
		c.rpcOK(t, ipc.MethodSessionPause, nil)
		elapsed := time.Since(start)

		if elapsed > 1*time.Second {
			t.Fatalf("session.pause took %v; want non-blocking despite Overlay.Show blocked", elapsed)
		}

		changes := c.waitChanges(t, 2) // running, paused
		if len(changes) < 2 || changes[1].State != string(domain.SessionPaused) {
			t.Errorf("session state = %v; want paused", states(changes))
		}
	})
}

func TestComponentFailures(t *testing.T) {
	t.Run("PAU-006/fonte com Start que falha: Warn com source e error, o agente sobe e as outras fontes funcionam", func(t *testing.T) {
		failingSrc := newFakeSource("failing-source", errors.New("device open failed"))
		workingSrc := newFakeSource("working-source", nil)

		var cfgRef config
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfgRef = *cfg
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						Sources: []input.Source{failingSrc, workingSrc},
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Fallback Game")
		w.launch(t, c, gameID)

		// Trigger pause via working source.
		w.clock.Advance(100 * time.Millisecond)
		workingSrc.emit(input.Trigger{Source: domain.PauseHotkey, At: w.clock.Now()})

		changes := c.waitChanges(t, 2)
		if len(changes) < 2 || changes[1].State != string(domain.SessionPaused) {
			t.Errorf("session state = %v; want paused", states(changes))
		}

		// Check warning log for failing source
		var foundWarn bool
		for _, rec := range logLines(t, agentLogPath(cfgRef)) {
			if rec["level"] == "WARN" && rec["msg"] == "fonte de gatilho de pausa indisponível" {
				if rec["source"] == "failing-source" {
					foundWarn = true
					break
				}
			}
		}
		if !foundWarn {
			t.Error("expected WARN log for failing-source not found")
		}
	})

	t.Run("PAU-006/energia que falha ao iniciar não impede o agente", func(t *testing.T) {
		var cfgRef config
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfgRef = *cfg
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						Power: func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error) {
							return nil, errors.New("power registration error")
						},
					}
				}
			},
		})

		c := w.client(t)
		_ = c.rpcOK(t, ipc.MethodStatus, nil)

		var foundWarn bool
		for _, rec := range logLines(t, agentLogPath(cfgRef)) {
			if rec["level"] == "WARN" && rec["msg"] == "monitor de energia indisponível" {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Error("expected WARN log for power failure not found")
		}
	})

	t.Run("PAU-006/NewOverlay com erro: Warn aviso na tela indisponível, o som e a pausa continuam", func(t *testing.T) {
		sound := &fakeSoundRecorder{}
		var cfgRef config
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfgRef = *cfg
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						NewOverlay: func() (feedback.Overlay, error) {
							return nil, errors.New("window class already registered")
						},
						NewSound: func() (feedback.Sound, error) {
							return sound, nil
						},
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Overlay Err Game")
		w.launch(t, c, gameID)

		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		sound.mu.Lock()
		cues := len(sound.cues)
		sound.mu.Unlock()
		if cues == 0 {
			t.Error("expected sound to play despite overlay error, got 0 cues")
		}

		var foundWarn bool
		for _, rec := range logLines(t, agentLogPath(cfgRef)) {
			if rec["level"] == "WARN" && rec["msg"] == "aviso na tela indisponível" {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Error("expected WARN log for overlay error not found")
		}
	})

	t.Run("PAU-006/NewSound com erro: o aviso e a pausa continuam", func(t *testing.T) {
		overlay := &blockingOverlay{}
		var cfgRef config
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfgRef = *cfg
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						NewOverlay: func() (feedback.Overlay, error) { return overlay, nil },
						NewSound: func() (feedback.Sound, error) {
							return nil, errors.New("sound device busy")
						},
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Sound Err Game")
		w.launch(t, c, gameID)

		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		overlay.mu.Lock()
		shows := len(overlay.shows)
		overlay.mu.Unlock()
		if shows == 0 {
			t.Error("expected overlay to show despite sound error, got 0 shows")
		}

		var foundWarn bool
		for _, rec := range logLines(t, agentLogPath(cfgRef)) {
			if rec["level"] == "WARN" && rec["msg"] == "som de feedback indisponível" {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Error("expected WARN log for sound error not found")
		}
	})

	t.Run("PAU-006/o rastreamento de tempo não muda: lançar, pausar e encerrar funciona com tudo indisponível", func(t *testing.T) {
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						Sources: []input.Source{newFakeSource("fail", errors.New("err"))},
						Power: func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error) {
							return nil, errors.New("err")
						},
						NewOverlay: func() (feedback.Overlay, error) { return nil, errors.New("err") },
						NewSound:   func() (feedback.Sound, error) { return nil, errors.New("err") },
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "All Fail Game")
		w.launch(t, c, gameID)

		w.clock.Advance(35 * time.Second)
		c.rpcOK(t, ipc.MethodSessionPause, nil)

		c.rpcOK(t, ipc.MethodSessionResume, nil)

		w.fl.job(t, 0).empty(t)
		_ = c.waitChanges(t, 4) // running, paused, running, grace
		w.clock.Advance(domain.SessionGracePeriod)

		changes := c.waitChanges(t, 5) // running, paused, running, grace, ended
		want := []string{"running", "paused", "running", "grace", "ended"}
		if got := states(changes); !slicesEqual(got, want) {
			t.Errorf("states = %v, want %v", got, want)
		}

		g, err := w.a.store.Game(context.Background(), gameID)
		if err != nil {
			t.Fatalf("store.Game: %v", err)
		}
		if g.TotalPlaySeconds != 35 {
			t.Errorf("TotalPlaySeconds = %d; want 35", g.TotalPlaySeconds)
		}
	})
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
