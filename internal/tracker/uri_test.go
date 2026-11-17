package tracker_test

import (
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// uriSpec is a game the shell opens (RUL-003).
var uriSpec = tracker.LaunchSpec{
	GameID: testGameID,
	Title:  "Counter-Strike 2",
	Exe:    "steam://rungameid/730",
	Kind:   tracker.KindURI,
}

func TestTrackerURILaunch(t *testing.T) {
	t.Parallel()

	t.Run("RUL-003/Launch uri abre o URI, grava a sessão e não cria job", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, err := e.tryLaunch(t, uriSpec)
		if err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		if got := e.launcher.urisOpened(); len(got) != 1 || got[0] != uriSpec.Exe {
			t.Errorf("OpenURI calls = %v, want [%s]", got, uriSpec.Exe)
		}
		if n := e.rec.count(opStart); n != 1 {
			t.Errorf("StartSession calls = %d, want 1", n)
		}
		if n := e.rec.count(opPrepare) + e.launcher.jobCount(); n != 0 {
			t.Errorf("Prepare or job created = %d, want none", n)
		}
		if snap.ID != firstSessionID {
			t.Errorf("Launch snapshot id = %d, want %d", snap.ID, firstSessionID)
		}
		if snap.State != domain.SessionRunning || snap.Active != 0 {
			t.Errorf("Launch snapshot state = %q active = %v, want running and 0 (the UI rejects an empty state)",
				snap.State, snap.Active)
		}
	})

	t.Run("RUL-003/falha ao abrir o URI não cria sessão", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launcher.openURIErr = errors.New("no handler for steam://")
		if _, err := e.tryLaunch(t, uriSpec); err == nil {
			t.Fatal("Launch with a failing shell open returned nil error")
		}
		if n := e.rec.count(opStart); n != 0 {
			t.Errorf("StartSession calls = %d, want 0", n)
		}
		if _, ok := e.tr.Current(); ok {
			t.Error("a session is current after the URI failed")
		}
	})

	t.Run("RUL-003/sem processo no prazo a sessão termina como discarded com client-no-launch", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		e.advance(t, 1, domain.SessionAdoptTimeout)
		ends := e.rec.waitN(t, opEnd, 1)
		if ends[0].end.Reason != domain.EndClientNoLaunch || ends[0].end.Active != 0 {
			t.Errorf("end = reason %q active %v, want client-no-launch and 0", ends[0].end.Reason, ends[0].end.Active)
		}
		n := e.nextNotify(t)
		if n.State != domain.SessionDiscarded {
			t.Errorf("notification state = %q, want discarded", n.State)
		}
		if _, ok := e.tr.Current(); ok {
			t.Error("a session is still current after the adoption timeout")
		}
	})
}
