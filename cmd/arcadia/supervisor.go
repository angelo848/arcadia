package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const (
	// relaunchMaxFailures is how many consecutive failures make the UI give up
	// relaunching the agent (EXE-022).
	relaunchMaxFailures = 3
	// stableConnection is the minimum connection age for a drop not to count
	// as a failure; a drop after it resets the counter (EXE-023).
	stableConnection = 60 * time.Second
	// firstDialTimeout is how long, after a successful Ensure, the client may
	// take to connect before it is treated as a drop (tasks.md, decision 4).
	firstDialTimeout = 5 * time.Second
)

// relaunchWaits are the waits before a relaunch attempt, indexed by the number
// of consecutive failures (EXE-022).
var relaunchWaits = [relaunchMaxFailures]time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// connectGate is the part of the library gate the supervisor opens (DSK-007).
type connectGate interface{ MarkConnected() }

// supervisorConfig is everything the supervisor needs, injected for tests.
type supervisorConfig struct {
	Clock domain.Clock
	// Ensure probes the agent and starts it when it does not answer (EXE-008).
	Ensure func(ctx context.Context) (ipc.AgentStatus, error)
	// NewClient creates the long-lived IPC client (reconnecting on its own).
	NewClient func() (ipcClient, error)
	// Publish hands the client to its owner; false means the owner is shutting
	// down and the supervisor must close the client and return.
	Publish func(c ipcClient) bool
	Gate    connectGate
	Root    string
	// Emit forwards an event to the frontend.
	Emit func(event string, data any)
	// Quit asks the UI to close (EXE-017).
	Quit func()
	Log  *slog.Logger
}

// supState is the supervisor state (design.md, transition table). Attempting
// is not a state of its own: Ensure runs inline on the supervisor goroutine.
type supState int

const (
	stateConnected supState = iota
	stateWaiting
	stateGaveUp
	stateQuitting
)

// supervisor keeps the agent running for the UI: it relaunches it after a
// drop without agent.shutdown (EXE-020..EXE-023), closes the UI on
// agent.shutdown (EXE-017) and forwards heartbeats (DSK-003). Every field but
// kick is owned by the run goroutine.
type supervisor struct {
	cfg  supervisorConfig
	kick chan struct{}

	client     ipcClient
	state      supState
	failures   int
	upSince    time.Time
	gateMarked bool
	timer      <-chan time.Time // relaunch wait (Waiting only)
	firstDial  <-chan time.Time // decision 4: no connection after Ensure ok
}

func newSupervisor(cfg supervisorConfig) *supervisor {
	return &supervisor{cfg: cfg, kick: make(chan struct{}, 1)}
}

// Kick asks for an immediate attempt when the supervisor gave up (EXE-012). It
// never blocks; it is handled only by the run loop.
func (s *supervisor) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// run owns the supervision loop. It returns when ctx is done or when the
// client's Notifications() channel is closed (after handling what was already
// buffered), or at once when Publish refuses the client.
func (s *supervisor) run(ctx context.Context) {
	st, err := s.cfg.Ensure(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		// EXE-024: a failed startup counts as the 1st failure.
		s.failures = 1
		s.logFailure(err)
		s.state = stateWaiting
	} else {
		s.connectedByEnsure(st)
	}

	c, err := s.cfg.NewClient()
	if err != nil {
		s.cfg.Log.Error("falha ao criar o cliente IPC", "error", err.Error())
		return
	}
	if !s.cfg.Publish(c) {
		if err := c.Close(); err != nil {
			s.cfg.Log.Warn("falha ao fechar o cliente IPC", "error", err.Error())
		}
		return
	}
	s.client = c
	switch s.state {
	case stateWaiting:
		s.timer = s.cfg.Clock.After(relaunchWaits[s.failures])
	case stateConnected:
		s.armFirstDial()
	}

	notes := c.Notifications()
	states := c.StateChanged()
	for {
		alive := true
		select {
		case <-ctx.Done():
			return
		case n, ok := <-notes:
			if !ok {
				return
			}
			s.handleNote(ctx, n)
		case _, ok := <-states:
			if !ok {
				states = nil // closed with Notifications(); the loop exits there
				continue
			}
			alive = s.onState(ctx, notes)
		case <-s.timer:
			s.timer = nil
			alive = s.onRetryTimer(ctx)
		case <-s.firstDial:
			s.firstDial = nil
			if s.client.Connected() {
				// The dial and the timer were ready together (the client's
				// backoff reaches firstDialTimeout): it is a connection, not a
				// drop, handled like StateChanged arriving connected.
				s.onConnected(ctx)
				continue
			}
			alive = s.onDrop(ctx, notes)
		case <-s.kick:
			alive = s.onKick(ctx)
		}
		if !alive {
			return
		}
	}
}

// connectedByEnsure records a successful Ensure: Connected, upSince = now,
// gate and data dir check on the first connection (DSK-006, DSK-007).
func (s *supervisor) connectedByEnsure(st ipc.AgentStatus) {
	s.state = stateConnected
	s.upSince = s.cfg.Clock.Now()
	if !s.gateMarked {
		s.gateMarked = true
		s.cfg.Gate.MarkConnected()
		warnIfDataDirDiffers(s.cfg.Log, s.cfg.Root, st.DataDir)
	}
}

// armFirstDial starts the decision-4 timer when the client is not connected
// yet after a successful Ensure.
func (s *supervisor) armFirstDial() {
	if s.client != nil && !s.client.Connected() {
		s.firstDial = s.cfg.Clock.After(firstDialTimeout)
	}
}

// markGateLate is the first connection of DSK-007 when Ensure did not reach
// the agent: it opens the gate and checks the data dir once.
func (s *supervisor) markGateLate(ctx context.Context) {
	if s.gateMarked {
		return
	}
	s.gateMarked = true
	s.cfg.Gate.MarkConnected()
	cctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	var st ipc.AgentStatus
	if err := s.client.Call(cctx, ipc.MethodStatus, nil, &st); err != nil {
		s.cfg.Log.Warn("falha ao consultar agent.status", "error", err.Error())
		return
	}
	warnIfDataDirDiffers(s.cfg.Log, s.cfg.Root, st.DataDir)
}

// handleNote handles one notification from the agent.
func (s *supervisor) handleNote(ctx context.Context, n ipc.Notification) {
	switch n.Method {
	case ipc.MethodHeartbeat:
		s.markGateLate(ctx)
		if len(n.Params) == 0 {
			s.cfg.Log.Warn("heartbeat com parâmetros inválidos", "error", "params ausentes")
			return
		}
		var hb ipc.HeartbeatParams
		if err := json.Unmarshal(n.Params, &hb); err != nil {
			s.cfg.Log.Warn("heartbeat com parâmetros inválidos", "error", err.Error())
			return
		}
		s.cfg.Emit(heartbeatEvent, hb)
	case ipc.MethodShutdown:
		if s.state == stateQuitting {
			return
		}
		var p ipc.ShutdownParams
		if len(n.Params) > 0 {
			if err := json.Unmarshal(n.Params, &p); err != nil {
				s.cfg.Log.Debug("agent.shutdown com parâmetros inválidos", "error", err.Error())
			}
		}
		s.cfg.Log.Info("agente encerrado; fechando a UI", "reason", p.Reason)
		s.state = stateQuitting
		s.timer, s.firstDial = nil, nil
		s.cfg.Quit()
	default:
		s.cfg.Log.Debug("notificação ignorada", "method", n.Method)
	}
}

// onState handles a StateChanged signal; it reads Connected() after it
// (EXE-019). It returns false when run must return.
func (s *supervisor) onState(ctx context.Context, notes <-chan ipc.Notification) bool {
	if s.state == stateQuitting {
		return true
	}
	if s.client.Connected() {
		s.onConnected(ctx)
		return true
	}
	if s.state == stateConnected {
		return s.onDrop(ctx, notes)
	}
	return true
}

// onConnected handles the client being connected: it clears the decision-4
// timer, opens the gate on the first connection (DSK-007) and leaves Waiting
// or GaveUp (EXE-021, EXE-022). In Connected, upSince stays the Ensure time.
func (s *supervisor) onConnected(ctx context.Context) {
	if s.state == stateQuitting {
		return
	}
	s.firstDial = nil
	s.markGateLate(ctx)
	if s.state == stateWaiting || s.state == stateGaveUp {
		s.reconnected()
	}
}

// reconnected is the connection coming back by itself (EXE-021, EXE-022).
func (s *supervisor) reconnected() {
	s.state = stateConnected
	s.upSince = s.cfg.Clock.Now()
	s.timer = nil
	s.cfg.Log.Info("agente reconectado")
}

// drainNotes handles, without blocking, the notifications already buffered;
// it returns false when the channel was closed.
func (s *supervisor) drainNotes(ctx context.Context, notes <-chan ipc.Notification) bool {
	for {
		select {
		case n, ok := <-notes:
			if !ok {
				return false
			}
			s.handleNote(ctx, n)
		default:
			return true
		}
	}
}

// onDrop handles a drop of a connection (or decision 4) in Connected. A
// buffered agent.shutdown wins over the drop (EXE-017, EXE-019).
func (s *supervisor) onDrop(ctx context.Context, notes <-chan ipc.Notification) bool {
	s.firstDial = nil
	if !s.drainNotes(ctx, notes) {
		return false
	}
	if s.state != stateConnected {
		return true
	}
	if s.cfg.Clock.Now().Sub(s.upSince) < stableConnection {
		s.failures++
		s.cfg.Log.Warn("agente caiu logo após subir")
	} else {
		s.failures = 0
	}
	if s.failures >= relaunchMaxFailures {
		s.cfg.Log.Warn("agente desconectado sem aviso de encerramento")
		s.giveUp()
		return true
	}
	wait := relaunchWaits[s.failures]
	s.cfg.Log.Warn("agente desconectado sem aviso de encerramento", "retry_in", wait.String())
	s.state = stateWaiting
	s.timer = s.cfg.Clock.After(wait)
	return true
}

// onRetryTimer is the end of a relaunch wait (EXE-020).
func (s *supervisor) onRetryTimer(ctx context.Context) bool {
	if s.state != stateWaiting {
		return true
	}
	if s.client.Connected() {
		s.markGateLate(ctx)
		s.reconnected()
		return true
	}
	return s.attempt(ctx)
}

// onKick is the second-instance notice (EXE-012): only GaveUp reacts.
func (s *supervisor) onKick(ctx context.Context) bool {
	if s.state != stateGaveUp {
		return true
	}
	s.failures = 0
	return s.attempt(ctx)
}

// attempt runs one relaunch attempt on the supervisor goroutine. It returns
// false when ctx ended during Ensure (EXE-025).
func (s *supervisor) attempt(ctx context.Context) bool {
	s.timer = nil
	s.cfg.Log.Info("relançando o agente", "attempt", s.failures+1, "max", relaunchMaxFailures)
	st, err := s.cfg.Ensure(ctx)
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		s.connectedByEnsure(st)
		s.cfg.Log.Info("agente reconectado")
		s.armFirstDial()
		return true
	}
	s.failures++
	s.logFailure(err)
	if s.failures >= relaunchMaxFailures {
		s.giveUp()
		return true
	}
	s.state = stateWaiting
	s.timer = s.cfg.Clock.After(relaunchWaits[s.failures])
	return true
}

func (s *supervisor) logFailure(err error) {
	s.cfg.Log.Error("falha ao relançar o agente", "attempt", s.failures, "error", err.Error())
}

// giveUp stops relaunching; the client keeps reconnecting on its own and a
// connection that comes back is still detected (EXE-022).
func (s *supervisor) giveUp() {
	s.state = stateGaveUp
	s.timer = nil
	s.cfg.Log.Error("desistindo de relançar o agente")
}
