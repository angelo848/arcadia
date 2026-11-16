package agent

// Tests for T14 of docs/specs/phase-13-modo-console/tasks.md: the wiring of the close-by-hotkey track in
// app.go [CON-003, CON-012, CON-014, CON-017, CON-019, CON-021]. The pad is a fakeSource that delivers
// Trigger{ActionClose}; the tracker is the real one with a fake launcher and a fake job; the sound is a
// recorder behind the real feedback coordinator. No test sleeps: waits are channels with testTimeout.
//
// Assumed production seams (not in the code yet):
//   - instance.closeNotifier *gameClosedNotifier, bound to the IPC server at startup and set(nil) at the
//     end of Shutdown (after a.wg.Wait());
//   - Shutdown calls a.step("router.close") right after the router is closed (before the drain).

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
	"log/slog"
)

// cueSound is a feedback.Sound that reports every cue on a channel.
type cueSound struct{ cues chan feedback.Cue }

func newCueSound() *cueSound { return &cueSound{cues: make(chan feedback.Cue, 16)} }

func (s *cueSound) Play(c feedback.Cue) {
	select {
	case s.cues <- c:
	default:
	}
}
func (s *cueSound) Close() error { return nil }

// closingLauncher wraps the fakeLauncher with jobs that accept the close request (RequestClose reports
// one window), so a close by hotkey waits for the game, and that count their Kill calls.
type closingLauncher struct {
	*fakeLauncher
	asked chan struct{} // one value per RequestClose
	kills *atomic.Int32
}

func (l closingLauncher) Prepare(spec tracker.LaunchSpec) (tracker.Job, error) {
	j, err := l.fakeLauncher.Prepare(spec)
	if err != nil {
		return nil, err
	}
	return &waitingJob{fakeJob: j.(*fakeJob), l: l}, nil
}

type waitingJob struct {
	*fakeJob
	l closingLauncher
}

func (j *waitingJob) RequestClose() (int, error) {
	select {
	case j.l.asked <- struct{}{}:
	default:
	}
	return 1, nil
}

func (j *waitingJob) Kill() error {
	j.l.kills.Add(1)
	return nil
}

// padOpts is the mutate of the agent under test: a fake pad source and the recorded sound.
func padOpts(src *fakeSource, snd feedback.Sound, launcher func(cfg *config)) wiringOpts {
	return wiringOpts{mutate: func(cfg *config) {
		cfg.Platform = func(domain.Clock, *slog.Logger) platformParts {
			return platformParts{
				Sources:  []input.Source{src},
				NewSound: func() (feedback.Sound, error) { return snd, nil },
			}
		}
		if launcher != nil {
			launcher(cfg)
		}
	}}
}

func (s *cueSound) none(t *testing.T) {
	t.Helper()
	select {
	case c := <-s.cues:
		t.Errorf("sound cue %v played, want none", c)
	default:
	}
}

func TestCloseByHotkeyWiring(t *testing.T) {
	t.Run("CON-019/ponta a ponta: pad falso com Back+B, tracker real com launcher falso e job falso: a sessão termina com closed-by-hotkey, o cliente IPC recebe session.changed e depois game.closedByHotkey", func(t *testing.T) {
		src, snd := newFakeSource("pad", nil), newCueSound()
		w := newWiring(t, padOpts(src, snd, nil))
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Fechar")
		launched := w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(120 * time.Second)
		at := w.clock.Now()

		src.emit(input.Trigger{Source: domain.PauseGamepad, Action: input.ActionClose, At: at})
		all := c.waitAll(t, "game.closedByHotkey", func(all []rawMsg) bool {
			return countMethod(all, ipc.MethodGameClosedByHotkey) == 1
		})

		sessAt, noteAt := -1, -1
		for i, m := range all {
			switch m.method() {
			case ipc.MethodSessionChanged:
				sessAt = i // the last one
			case ipc.MethodGameClosedByHotkey:
				noteAt = i
				if string(m["params"]) != fmt.Sprintf(`{"gameId":%d}`, gameID) {
					t.Errorf("params = %s, want {gameId: %d}", m["params"], gameID)
				}
			}
		}
		if sessAt < 0 || sessAt > noteAt {
			t.Errorf("last session.changed at %d, game.closedByHotkey at %d; want session.changed first", sessAt, noteAt)
		}
		got := sessionChanges(t, all)
		if len(got) != 2 || got[1].State != string(domain.SessionEnded) || got[1].SessionID != launched.SessionID {
			t.Errorf("session.changed = %+v, want running then ended of session %d", got, launched.SessionID)
		}

		t.Run("CON-017/session.list devolve endReason closed-by-hotkey para a sessão fechada", func(t *testing.T) {
			var list ipc.SessionListResult
			resultInto(t, c.rpcOK(t, ipc.MethodSessionList, ipc.SessionListParams{GameID: gameID}), &list)
			if len(list.Sessions) != 1 {
				t.Fatalf("sessions = %+v, want 1", list.Sessions)
			}
			s := list.Sessions[0]
			if s.EndReason == nil || *s.EndReason != "closed-by-hotkey" {
				t.Errorf("endReason = %v, want closed-by-hotkey", s.EndReason)
			}
			if s.EndedAt == nil || !s.EndedAt.Equal(at) || s.ActiveSeconds != 120 {
				t.Errorf("session = %+v, want endedAt %v and 120 active seconds", s, at)
			}
		})

		t.Run("CON-014/sucesso não envia feedback", func(t *testing.T) {
			c.rpcOK(t, ipc.MethodStatus, nil)
			snd.none(t)
		})
	})

	t.Run("CON-012/Back+B sem jogo: o som de recusa é pedido ao coordenador de feedback e nada é gravado", func(t *testing.T) {
		src, snd := newFakeSource("pad", nil), newCueSound()
		w := newWiring(t, padOpts(src, snd, nil))
		c := w.client(t)

		src.emit(input.Trigger{Source: domain.PauseGamepad, Action: input.ActionClose, At: w.clock.Now()})
		select {
		case cue := <-snd.cues:
			if cue != feedback.CueRefused {
				t.Errorf("cue = %v, want CueRefused", cue)
			}
		case <-time.After(testTimeout):
			t.Fatal("no refusal sound within the timeout")
		}
		wantRaw(t, objectOf(t, c.rpcOK(t, ipc.MethodSessionCurrent, nil).Result), "session", "null")
		if n := countMethod(c.snapshotAll(), ipc.MethodGameClosedByHotkey); n != 0 {
			t.Errorf("game.closedByHotkey received %d time(s), want 0", n)
		}
		w.shutdown(t)
		if rows, _ := sessionsAfterShutdown(t, w.cfg); len(rows) != 0 {
			t.Errorf("play_session has %d row(s), want 0", len(rows))
		}
	})

	t.Run("CON-021/Shutdown com fechamento em espera termina dentro do orçamento, sem goroutine restante (-race), sem matar o jogo", func(t *testing.T) {
		src, snd := newFakeSource("pad", nil), newCueSound()
		cl := closingLauncher{asked: make(chan struct{}, 4), kills: &atomic.Int32{}}
		w := newWiring(t, padOpts(src, snd, func(cfg *config) {
			cl.fakeLauncher = cfg.Launcher.(*fakeLauncher)
			cfg.Launcher = cl
		}))
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Em espera")
		launched := w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(100 * time.Second)
		at := w.clock.Now()

		src.emit(input.Trigger{Source: domain.PauseGamepad, Action: input.ActionClose, At: at})
		select {
		case <-cl.asked: // the session is in Closing, waiting for the game (the FakeClock never moves)
		case <-time.After(testTimeout):
			t.Fatal("the close request never reached the job")
		}

		w.a.drainTimeout = 50 * time.Millisecond
		start := time.Now()
		w.shutdown(t)
		if took := time.Since(start); took >= 2*time.Second {
			t.Errorf("Shutdown with a close in wait took %v, want < 2s", took)
		}

		rows, _ := sessionsAfterShutdown(t, w.cfg)
		wantEnded(t, rows[launched.SessionID], string(domain.SessionEnded), at, 100, domain.EndAgentShutdown)
		if k := cl.kills.Load(); k != 0 {
			t.Errorf("Job.Kill calls = %d, want 0 (the game is not killed at shutdown)", k)
		}
		if _, terminates, _ := w.fl.job(t, 0).counts(); terminates != 0 {
			t.Errorf("Job.Terminate calls = %d, want 0", terminates)
		}
		msgs := c.waitEOF(t)
		if n := countMethod(msgs, ipc.MethodGameClosedByHotkey); n != 0 {
			t.Errorf("game.closedByHotkey received %d time(s) at shutdown, want 0", n)
		}
		snd.none(t)
	})

	t.Run("CON-021/a ordem do Shutdown continua router.Close antes do drain, tracker.close depois do server.close; closeNotifier ligado e depois zerado", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		if w.a.closeNotifier == nil || w.a.closeNotifier.srv.Load() == nil {
			t.Fatal("closeNotifier is not bound to the IPC server after startAgent")
		}
		var (
			mu    sync.Mutex
			steps []string
		)
		w.a.onShutdownStep = func(s string) {
			mu.Lock()
			defer mu.Unlock()
			steps = append(steps, s)
		}
		w.shutdown(t)
		mu.Lock()
		got := append([]string(nil), steps...)
		mu.Unlock()
		router, server, trk := indexOf(got, "router.close"), indexOf(got, "server.close"), indexOf(got, "tracker.close")
		if router == -1 || server == -1 || trk == -1 || !(router < server && server < trk) {
			t.Errorf("shutdown steps = %v, want router.close < server.close < tracker.close", got)
		}
		if p := w.a.closeNotifier.srv.Load(); p != nil {
			t.Errorf("closeNotifier still bound to the IPC server after Shutdown: %p, want nil", p)
		}
	})
}
