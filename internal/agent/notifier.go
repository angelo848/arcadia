package agent

import (
	"sync/atomic"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// sessionNotifier bridges the tracker to the IPC server (the tracker is built
// before the server exists) [IPC-027]. Before set, and after set(nil), Notify
// is a no-op; it never blocks (ipc.Server.Notify drops on a full queue).
type sessionNotifier struct{ srv atomic.Pointer[ipc.Server] }

// mediaNotifier bridges the media queue to the IPC server [IMG-001, IMG-002].
// Before set, and after set(nil), MediaChanged is a no-op; it never blocks.
type mediaNotifier struct{ srv atomic.Pointer[ipc.Server] }

// set points the notifier at s (nil detaches it).
func (n *mediaNotifier) set(s *ipc.Server) { n.srv.Store(s) }

// MediaChanged broadcasts media.changed for gameID to every connected client.
func (n *mediaNotifier) MediaChanged(gameID int64) {
	srv := n.srv.Load()
	if srv == nil {
		return
	}
	srv.Notify(ipc.MethodMediaChanged, ipc.MediaChangedParams{GameID: gameID})
}

// batchNotifier bridges the metadata batch to the IPC server [MET-054]. Before set, and after
// set(nil), BatchChanged is a no-op; it never blocks (ipc.Server.Notify drops on a full queue).
type batchNotifier struct{ srv atomic.Pointer[ipc.Server] }

// set points the notifier at s (nil detaches it).
func (n *batchNotifier) set(s *ipc.Server) { n.srv.Store(s) }

// BatchChanged broadcasts metadata.batchChanged with p to every connected client.
func (n *batchNotifier) BatchChanged(p ipc.MetadataBatchChangedParams) {
	srv := n.srv.Load()
	if srv == nil {
		return
	}
	srv.Notify(ipc.MethodMetadataBatchChanged, p)
}

// gameClosedNotifier bridges the close router to the IPC server [CON-019]. Before set, and
// after set(nil), GameClosedByHotkey is a no-op; it never blocks.
type gameClosedNotifier struct{ srv atomic.Pointer[ipc.Server] }

// set points the notifier at s (nil detaches it).
func (n *gameClosedNotifier) set(s *ipc.Server) { n.srv.Store(s) }

// GameClosedByHotkey broadcasts game.closedByHotkey for gameID to every connected client.
func (n *gameClosedNotifier) GameClosedByHotkey(gameID int64) {
	srv := n.srv.Load()
	if srv == nil {
		return
	}
	srv.Notify(ipc.MethodGameClosedByHotkey, struct {
		GameID int64 `json:"gameId"`
	}{gameID})
}

// set points the notifier at s (nil detaches it).
func (n *sessionNotifier) set(s *ipc.Server) { n.srv.Store(s) }

// Notify broadcasts session.changed with s to every connected client.
func (n *sessionNotifier) Notify(s domain.SessionSnapshot) {
	srv := n.srv.Load()
	if srv == nil {
		return
	}
	srv.Notify(ipc.MethodSessionChanged, ipc.SessionChangedParams{Session: ipc.SessionFromSnapshot(s)})
}
