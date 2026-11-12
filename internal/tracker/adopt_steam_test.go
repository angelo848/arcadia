package tracker_test

import (
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// fakeSteam is a scripted Steam Running signal (STM-001).
type fakeSteam struct {
	mu      sync.Mutex
	running bool
	present bool
}

func (s *fakeSteam) set(running, present bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running, s.present = running, present
}

func (s *fakeSteam) read(string) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.present, nil
}

func TestTrackerSteamSignal(t *testing.T) {
	t.Parallel()

	t.Run("STM-001/Running 0 depois de adotado encerra a sessão sem carência", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc)
		steam := &fakeSteam{}
		steam.set(false, true)
		e := newEnv(t, envConfig{
			heartbeat: time.Hour,
			poll:      time.Hour,
			procs:     lister,
			watch:     watchEvery,
			rules:     gameRules,
			steam:     steam.read,
		})
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(steam uri): %v", err)
		}
		// Steam starts the game: Running goes 0 -> 1, then the game process appears.
		steam.set(true, true)
		lister.set(explorerProc, gameProc)
		tick(t, e, 1)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionRunning {
			t.Fatalf("first notification = %q, want running", got[0])
		}
		// Steam stops the game: Running goes back to 0. The session goes to grace
		// at once, and ends without the 15 s grace period.
		steam.set(false, true)
		lister.set(explorerProc)
		tick(t, e, 2)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionGrace {
			t.Errorf("second notification = %q, want grace", got[0])
		}
		// The zero-delay grace timer is armed; the fake clock fires it on any advance.
		e.clock.Advance(time.Nanosecond)
		end := e.rec.waitN(t, opEnd, 1)[0].end
		if end.EndedAt.IsZero() {
			t.Error("the session was not ended at the Steam stop")
		}
	})
}

// trackingOf reports the tracking mode of the current session (RUL-005).
func trackingOf(e *env) domain.TrackingMode {
	cur, ok := e.tr.Current()
	if !ok {
		return ""
	}
	return cur.Tracking
}
