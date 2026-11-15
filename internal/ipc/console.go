package ipc

// MethodGameClosedByHotkey is the server-to-client notification sent after the
// gamepad combo closed a game (contract v0.15.0) [CON-019].
const MethodGameClosedByHotkey = "game.closedByHotkey"

// GameClosedByHotkeyParams are the params of game.closedByHotkey.
type GameClosedByHotkeyParams struct {
	GameID int64 `json:"gameId"`
}
