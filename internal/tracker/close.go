package tracker

import (
	"context"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// CloseKind is the outcome of CloseByHotkey (contracts/session-close-state-machine.md).
type CloseKind int

// CloseByHotkey outcomes.
const (
	CloseClosed  CloseKind = iota + 1 // the game was closed and its session ended
	CloseRefused                      // nothing to close; Reason says why
	CloseFailed                       // the game could not be killed; the session goes on (CON-013)
)

// Reasons of a CloseRefused result.
const (
	CloseReasonNoGame     = "no-game"     // no session (or a trigger from before it started) [CON-012]
	CloseReasonNotAdopted = "not-adopted" // URI launch whose game was not adopted yet [CON-009]
	CloseReasonClosing    = "closing"     // a close is already in progress [CON-032]
)

// CloseResult is the answer of CloseByHotkey.
type CloseResult struct {
	Kind      CloseKind
	GameID    int64
	Untracked bool   // the game closed was launched without counting time (T12)
	Reason    string // CloseRefused only
}

const (
	// closeWait is how long the game has to close after WM_CLOSE before it is
	// killed (x-console.closeWaitMs, CON-030).
	closeWait = 3 * time.Second
	// detachedPollEvery is the period of the check of an untracked game being
	// closed (x-console.closeDetachedPollMs, CON-010; used by T12).
	detachedPollEvery = 250 * time.Millisecond
)

// pendingClose is a close by hotkey in progress. It is owned by the run
// goroutine; reply has buffer 1 so the owner never blocks on it.
type pendingClose struct {
	at        time.Time
	reply     chan ctrlResult
	gen       uint64 // generation of the armed CloseTimeout (and DetachedPoll); 0 = none armed
	gameID    int64
	title     string // untracked only: reported by a launch refused during the wait (CON-031)
	untracked bool   // the game being closed is the kept detached job (CON-010)
}

// CloseByHotkey asks the game in progress to close (WM_CLOSE), waits up to
// closeWait on the Clock for its tree to end, kills what is left and ends its
// session at `at` with reason closed-by-hotkey [CON-005..CON-009, CON-030]. The
// reply comes when the close is over; the owner goroutine keeps serving other
// requests during the wait. It never plays feedback [CON-014]. After
// BeginShutdown or Close it returns ErrShuttingDown [CON-021, CON-033]; if ctx
// ends first it returns ctx.Err() and the close still completes.
func (t *Tracker) CloseByHotkey(ctx context.Context, at time.Time) (CloseResult, error) {
	r, err := t.control(ctx, ctrlReq{kind: ctrlClose, at: at})
	if err != nil {
		return CloseResult{}, err
	}
	return r.close, nil
}

// handleClose applies CloseByHotkey in the owner goroutine. It answers req
// itself: at once, or when the wait started here ends.
func (t *Tracker) handleClose(req ctrlReq) {
	reply := func(r ctrlResult) { req.reply <- r }
	if t.isShuttingDown() {
		reply(ctrlResult{err: ErrShuttingDown})
		return
	}
	if t.closeReq != nil {
		reply(ctrlResult{close: CloseResult{Kind: CloseRefused, Reason: CloseReasonClosing}})
		return
	}
	state := t.m.State()
	gameID, sid, _, _ := t.m.Pending()
	switch {
	case state == StateClosing:
		// Abandoned at BeginShutdown: Stop will end it (CON-033).
		reply(ctrlResult{close: CloseResult{Kind: CloseRefused, Reason: CloseReasonClosing}})
	case (state == StateRunning || state == StatePaused) && !req.at.Before(t.m.startedAt):
		t.closeSession(req, gameID, sid)
	case state == StateGrace:
		t.closeGrace(req, gameID, sid)
	case state == StateLaunching:
		t.log.Debug("fechar ignorado", slog.Int64("session_id", sid), slog.String("reason", CloseReasonNotAdopted))
		reply(ctrlResult{close: CloseResult{Kind: CloseRefused, Reason: CloseReasonNotAdopted}})
	case state == StateIdle && t.detached != nil:
		// Without a session the trigger acts on the untracked game; with one,
		// the session comes first (CON-010, CON-021).
		t.closeDetached(req)
	default:
		t.log.Debug("fechar ignorado", slog.Int("state", int(state)), slog.Time("at", req.at))
		reply(ctrlResult{close: CloseResult{Kind: CloseRefused, Reason: CloseReasonNoGame}})
	}
}

// closeSession moves a Running or Paused session to Closing at the trigger
// instant and asks its windows to close; with no window to ask, the tree is
// killed at once [CON-005, CON-006, CON-030].
func (t *Tracker) closeSession(req ctrlReq, gameID, sid int64) {
	at := t.monotonic(req.at)
	t.step(t.work, func() []Effect { return t.m.CloseRequested(at) })
	t.closeReq = &pendingClose{at: at, reply: req.reply, gameID: gameID}

	n, err := 0, error(nil)
	if t.job != nil {
		n, err = t.job.RequestClose()
	}
	if err != nil || n == 0 {
		t.log.Debug("nenhuma janela recebeu o pedido de fechar", slog.Int64("session_id", sid),
			slog.Int("windows", n), slog.Any("error", err))
		t.killAndFinish(t.monotonic(t.clock.Now()))
		return
	}
	t.closeGen++
	t.closeReq.gen = t.closeGen
	t.closeTimer = timer{ch: t.clock.After(closeWait), gen: t.closeGen}
}

// closeGrace ends a session in Grace at graceSince once what may be left of the
// tree is killed; there is nothing to ask nor to wait for [CON-007].
func (t *Tracker) closeGrace(req ctrlReq, gameID, sid int64) {
	if err := t.killJob(); err != nil {
		t.log.Warn("falha ao fechar o jogo", slog.Int64("session_id", sid), slog.Any("error", err))
		req.reply <- ctrlResult{close: CloseResult{Kind: CloseFailed, GameID: gameID}}
		return
	}
	at := t.monotonic(req.at)
	t.step(t.work, func() []Effect { return t.m.CloseRequested(at) })
	req.reply <- ctrlResult{close: CloseResult{Kind: CloseClosed, GameID: gameID}}
}

// handleCloseTimeout kills the tree when the wait of the current generation
// ends [CON-030]; late fires of older generations are ignored.
func (t *Tracker) handleCloseTimeout(now time.Time, gen uint64) {
	c := t.closeReq
	if c == nil || gen == 0 || gen != c.gen || (!c.untracked && t.m.State() != StateClosing) {
		t.log.Debug("prazo do fechamento obsoleto", slog.Uint64("gen", gen))
		return
	}
	if t.isShuttingDown() {
		t.abandonClose()
		return
	}
	if c.untracked {
		t.killDetached()
		return
	}
	t.killAndFinish(now)
}

// closeExited ends a closing session whose tree left during the wait, with no
// kill [CON-030].
func (t *Tracker) closeExited(now time.Time) {
	if t.closeReq == nil {
		// Abandoned at BeginShutdown: Stop ends it as agent-shutdown (CON-033).
		t.log.Debug("jogo encerrou durante o desligamento")
		return
	}
	if t.isShuttingDown() {
		t.abandonClose()
		return
	}
	t.finishClose(now)
}

// abandonClose gives up a close in progress because the agent is shutting
// down: nothing is killed, the session stays in Closing until Stop ends it at
// the trigger instant with reason agent-shutdown, and the caller gets
// ErrShuttingDown [CON-033].
func (t *Tracker) abandonClose() {
	t.answerClose(ctrlResult{err: ErrShuttingDown})
}

// killAndFinish kills the tree of the closing session and ends it; when the
// kill fails the session returns to its origin state [CON-013].
func (t *Tracker) killAndFinish(now time.Time) {
	if err := t.killJob(); err != nil {
		gameID, sid, _, _ := t.m.Pending()
		t.log.Warn("falha ao fechar o jogo", slog.Int64("session_id", sid), slog.Any("error", err))
		t.step(t.work, t.m.CloseUndone)
		t.answerClose(ctrlResult{close: CloseResult{Kind: CloseFailed, GameID: gameID}})
		return
	}
	t.finishClose(now)
}

// finishClose ends the closing session at its trigger instant (EndSession,
// CloseJob, Notify with AsOf now) and only then answers closed [CON-005,
// CON-019].
func (t *Tracker) finishClose(now time.Time) {
	gameID, _, _, _ := t.m.Pending()
	t.step(t.work, func() []Effect { return t.m.CloseDone(now) })
	t.answerClose(ctrlResult{close: CloseResult{Kind: CloseClosed, GameID: gameID}})
}

// answerClose replies to the close in progress, if any, and forgets it and its
// timer (a late fire is dropped with the channel).
func (t *Tracker) answerClose(r ctrlResult) {
	if t.closeReq == nil {
		return
	}
	t.closeReq.reply <- r
	t.closeReq = nil
	t.closeTimer = timer{}
	t.detachedPoll = timer{}
}

// closeDetached closes the untracked game kept by the tracker (CON-010): an
// empty job is released and the trigger refused; otherwise its windows are
// asked to close and the tree is checked every detachedPollEvery until it ends
// or closeWait passes, when what is left is killed. Nothing is recorded and no
// session.changed is sent; a failed read of the job counts as alive.
func (t *Tracker) closeDetached(req ctrlReq) {
	gameID := t.detachedGame
	n, err := t.detached.ActiveProcesses()
	if err == nil && n == 0 {
		t.log.Debug("fechar ignorado", slog.Int64("game_id", gameID), slog.String("reason", CloseReasonNoGame))
		t.releaseDetached()
		req.reply <- ctrlResult{close: CloseResult{Kind: CloseRefused, Reason: CloseReasonNoGame}}
		return
	}
	if err != nil {
		t.log.Debug("falha ao ler os processos do jogo sem contar tempo", slog.Int64("game_id", gameID), slog.Any("error", err))
	}
	t.closeReq = &pendingClose{at: req.at, reply: req.reply, gameID: gameID, title: t.detachedTitle, untracked: true}
	posted, err := t.detached.RequestClose()
	if err != nil || posted == 0 {
		t.log.Debug("nenhuma janela recebeu o pedido de fechar", slog.Int64("game_id", gameID),
			slog.Int("windows", posted), slog.Any("error", err))
		t.killDetached()
		return
	}
	t.closeGen++
	gen := t.closeGen
	t.closeReq.gen = gen
	t.closeTimer = timer{ch: t.clock.After(closeWait), gen: gen}
	t.detachedPoll = timer{ch: t.clock.After(detachedPollEvery), gen: gen}
}

// handleDetachedPoll checks the untracked game being closed: an empty job ends
// the close without kill; otherwise the check is re-armed (CON-010). Fires of
// older generations are ignored.
func (t *Tracker) handleDetachedPoll(gen uint64) {
	c := t.closeReq
	if c == nil || !c.untracked || gen == 0 || gen != c.gen || t.detached == nil {
		t.log.Debug("conferência do fechamento obsoleta", slog.Uint64("gen", gen))
		return
	}
	n, err := t.detached.ActiveProcesses()
	if err == nil && n == 0 {
		if t.isShuttingDown() {
			t.abandonClose() // the job is released by shutdown (CON-033)
			return
		}
		t.finishDetached()
		return
	}
	if err != nil {
		t.log.Debug("falha ao ler os processos do jogo sem contar tempo", slog.Int64("game_id", c.gameID), slog.Any("error", err))
	}
	t.detachedPoll = timer{ch: t.clock.After(detachedPollEvery), gen: gen}
}

// killDetached kills the tree of the untracked game being closed and ends the
// close; when the kill fails the job stays kept and the close fails (CON-013).
func (t *Tracker) killDetached() {
	gameID := t.closeReq.gameID
	if err := t.detached.Kill(); err != nil {
		t.log.Warn("falha ao fechar o jogo", slog.Int64("game_id", gameID), slog.Any("error", err))
		t.answerClose(ctrlResult{close: CloseResult{Kind: CloseFailed, GameID: gameID, Untracked: true}})
		return
	}
	t.finishDetached()
}

// finishDetached releases the job of the untracked game that was closed and
// answers closed (CON-010).
func (t *Tracker) finishDetached() {
	gameID := t.closeReq.gameID
	t.releaseDetached()
	t.answerClose(ctrlResult{close: CloseResult{Kind: CloseClosed, GameID: gameID, Untracked: true}})
}

// untrackedBlocksLaunch applies "one game at a time" to a new launch (CON-031,
// CON-011): while the untracked game is being closed or its job still has live
// processes the launch is refused with sessionId 0; an empty job is released
// (never killed) so the launch can go on. A failed read releases the job too,
// with a Warn, so that a broken job never blocks every later launch.
func (t *Tracker) untrackedBlocksLaunch() error {
	if c := t.closeReq; c != nil && c.untracked {
		return &SessionActiveError{Session: domain.SessionSnapshot{GameID: c.gameID, Title: c.title}}
	}
	if t.detached == nil {
		return nil
	}
	n, err := t.detached.ActiveProcesses()
	switch {
	case err != nil:
		t.log.Warn("falha ao ler os processos do jogo sem contar tempo", slog.Int64("game_id", t.detachedGame), slog.Any("error", err))
	case n > 0:
		return &SessionActiveError{Session: domain.SessionSnapshot{GameID: t.detachedGame, Title: t.detachedTitle}}
	}
	t.releaseDetached()
	return nil
}

// releaseDetached closes the kept job of the untracked game without killing
// it (ADR 0005) and forgets it. Without one it is a no-op.
func (t *Tracker) releaseDetached() {
	if t.detached == nil {
		return
	}
	if err := t.detached.Close(); err != nil {
		t.log.Warn("falha ao fechar o job do jogo sem contar tempo", slog.Int64("game_id", t.detachedGame), slog.Any("error", err))
	}
	t.detached, t.detachedGame, t.detachedTitle = nil, 0, ""
}

// killJob kills the tree of the session; without a job there is nothing to kill.
func (t *Tracker) killJob() error {
	if t.job == nil {
		return nil
	}
	return t.job.Kill()
}

// applyClosers installs Config.Closer and Config.Killer in an adopted job; nil
// keeps the real one (test hook of CON-008).
func (t *Tracker) applyClosers(j *adoptedJob) {
	if t.closer == nil && t.killer == nil {
		return
	}
	j.mu.Lock()
	c, k := j.closer, j.killer
	j.mu.Unlock()
	if t.closer != nil {
		c = t.closer
	}
	if t.killer != nil {
		k = t.killer
	}
	j.setClosers(c, k)
}
