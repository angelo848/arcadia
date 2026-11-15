package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// metadataBatchEvent is the Wails event emitted when a metadata batch changes (MET-072).
const metadataBatchEvent = "metadata:batch"

// MetadataBatchChangedView is the payload of the metadata:batch event. It carries only the
// state and the counters; the UI reads the game data with MetadataBatchStatus (MET-055).
type MetadataBatchChangedView struct {
	BatchID int64  `json:"batchId"`
	State   string `json:"state"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Failed  int    `json:"failed"`
}

// batchStateKnown reports whether s is one of the five BatchState values.
func batchStateKnown(s string) bool {
	switch s {
	case ipc.BatchStateSearching, ipc.BatchStateReady, ipc.BatchStateCancelled,
		ipc.BatchStateApplying, ipc.BatchStateDone:
		return true
	}
	return false
}

// batchInt validates one JSON integer token: an integer of at least min and at most
// maxSafeGameID (a JS number represents it exactly).
func batchInt(name string, raw json.RawMessage, min int64) (int64, error) {
	tok := bytes.TrimSpace(raw)
	if len(tok) == 0 || string(tok) == "null" {
		return 0, fmt.Errorf("%s ausente", name)
	}
	if tok[0] != '-' && (tok[0] < '0' || tok[0] > '9') {
		return 0, fmt.Errorf("%s não é número", name)
	}
	r, ok := new(big.Rat).SetString(string(tok))
	if !ok || !r.IsInt() {
		return 0, fmt.Errorf("%s não é inteiro", name)
	}
	n := r.Num()
	if !n.IsInt64() || n.Int64() < min || n.Int64() > maxSafeGameID {
		return 0, fmt.Errorf("%s fora da faixa", name)
	}
	return n.Int64(), nil
}

// decodeBatchChanged validates metadata.batchChanged params (MET-054, MET-072). Any missing
// or invalid field refuses the whole notification.
func decodeBatchChanged(raw json.RawMessage) (MetadataBatchChangedView, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return MetadataBatchChangedView{}, errors.New("params ausentes")
	}
	var p struct {
		BatchID json.RawMessage `json:"batchId"`
		State   json.RawMessage `json:"state"`
		Done    json.RawMessage `json:"done"`
		Total   json.RawMessage `json:"total"`
		Failed  json.RawMessage `json:"failed"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return MetadataBatchChangedView{}, fmt.Errorf("params inválidos: %w", err)
	}
	batchID, err := batchInt("batchId", p.BatchID, 1)
	if err != nil {
		return MetadataBatchChangedView{}, err
	}
	var state string
	if tok := bytes.TrimSpace(p.State); len(tok) == 0 || string(tok) == "null" {
		return MetadataBatchChangedView{}, errors.New("state ausente")
	}
	if err := json.Unmarshal(p.State, &state); err != nil {
		return MetadataBatchChangedView{}, fmt.Errorf("state não é texto: %w", err)
	}
	if !batchStateKnown(state) {
		return MetadataBatchChangedView{}, fmt.Errorf("state desconhecido: %q", state)
	}
	done, err := batchInt("done", p.Done, 0)
	if err != nil {
		return MetadataBatchChangedView{}, err
	}
	total, err := batchInt("total", p.Total, 0)
	if err != nil {
		return MetadataBatchChangedView{}, err
	}
	failed, err := batchInt("failed", p.Failed, 0)
	if err != nil {
		return MetadataBatchChangedView{}, err
	}
	return MetadataBatchChangedView{
		BatchID: batchID, State: state, Done: int(done), Total: int(total), Failed: int(failed),
	}, nil
}
