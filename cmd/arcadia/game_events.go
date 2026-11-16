package main

import "encoding/json"

// gameClosedEvent is the Wails event emitted when the controller closed the running game (CON-020).
const gameClosedEvent = "game:closedByHotkey"

// GameClosedView is the payload of the game:closedByHotkey event.
type GameClosedView struct {
	GameID int64 `json:"gameId"`
}

// decodeGameClosed validates game.closedByHotkey params with the same rules as media.changed.
func decodeGameClosed(raw json.RawMessage) (GameClosedView, error) {
	v, err := decodeMediaChanged(raw)
	if err != nil {
		return GameClosedView{}, err
	}
	return GameClosedView{GameID: v.GameID}, nil
}
