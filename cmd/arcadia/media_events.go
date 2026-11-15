package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
)

// mediaChangedEvent is the Wails event emitted when a game's media changes.
const mediaChangedEvent = "media:changed"

// maxSafeGameID is the largest integer a JS number represents exactly.
const maxSafeGameID = 9007199254740991

// MediaChangedView is the payload of the media:changed event.
type MediaChangedView struct {
	GameID int64 `json:"gameId"`
}

// decodeMediaChanged validates media.changed params (IMG-005).
func decodeMediaChanged(raw json.RawMessage) (MediaChangedView, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return MediaChangedView{}, errors.New("params ausentes")
	}
	var p struct {
		GameID json.RawMessage `json:"gameId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return MediaChangedView{}, fmt.Errorf("params inválidos: %w", err)
	}
	tok := bytes.TrimSpace(p.GameID)
	if len(tok) == 0 {
		return MediaChangedView{}, errors.New("gameId ausente")
	}
	if tok[0] != '-' && (tok[0] < '0' || tok[0] > '9') {
		return MediaChangedView{}, errors.New("gameId não é número")
	}
	r, ok := new(big.Rat).SetString(string(tok))
	if !ok || !r.IsInt() {
		return MediaChangedView{}, errors.New("gameId não é inteiro")
	}
	n := r.Num()
	if n.Sign() < 1 || !n.IsInt64() || n.Int64() > maxSafeGameID || n.Int64() > math.MaxInt64 {
		return MediaChangedView{}, errors.New("gameId fora da faixa")
	}
	return MediaChangedView{GameID: n.Int64()}, nil
}
