package main

import (
	"encoding/json"
	"testing"
)

func TestDecodeMediaChanged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  json.RawMessage
		want int64
		ok   bool
	}{
		{"1", json.RawMessage(`{"gameId":1}`), 1, true},
		{"42", json.RawMessage(`{"gameId":42}`), 42, true},
		{"grande", json.RawMessage(`{"gameId":9007199254740991}`), 9007199254740991, true},
		{"ausente", nil, 0, false},
		{"vazio", json.RawMessage(``), 0, false},
		{"JSON inválido", json.RawMessage(`{bad`), 0, false},
		{"objeto vazio", json.RawMessage(`{}`), 0, false},
		{"null", json.RawMessage(`null`), 0, false},
		{"gameId null", json.RawMessage(`{"gameId":null}`), 0, false},
		{"zero", json.RawMessage(`{"gameId":0}`), 0, false},
		{"negativo", json.RawMessage(`{"gameId":-3}`), 0, false},
		{"fracionário", json.RawMessage(`{"gameId":1.5}`), 0, false},
		{"string", json.RawMessage(`{"gameId":"7"}`), 0, false},
		{"array", json.RawMessage(`[42]`), 0, false},
	}
	for _, tc := range cases {
		t.Run("IMG-005/decodeMediaChanged aceita inteiro ≥ 1 e recusa o resto/"+tc.name, func(t *testing.T) {
			t.Parallel()
			v, err := decodeMediaChanged(tc.raw)
			if tc.ok {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				if v.GameID != tc.want {
					t.Errorf("GameID = %d, want %d", v.GameID, tc.want)
				}
				return
			}
			if err == nil {
				t.Errorf("decodeMediaChanged(%s) = %+v, want an error", tc.raw, v)
			}
		})
	}
}
