package main

import (
	"encoding/json"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestPlatformSetEmulatorParams checks the params sent to platform.setEmulator: the
// contract's object, with emulatorId null to clear the default (EMU-002).
func TestPlatformSetEmulatorParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		emulatorID int64
		want       any
	}{
		{"EMU-002/escolher um emulador envia o id", 7, float64(7)},
		{"EMU-002/0 limpa o padrão e envia null", 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{results: map[string]any{ipc.MethodPlatformSetEmulator: ipc.PlatformResult{Platform: ipc.Platform{ID: 3}}}}
			a := &App{client: p, log: discardLog()}

			if _, err := a.PlatformSetEmulator(3, tc.emulatorID); err != nil {
				t.Fatalf("PlatformSetEmulator err = %v", err)
			}
			c := singleCall(t, p, ipc.MethodPlatformSetEmulator, libraryTimeout)
			var got map[string]any
			if err := json.Unmarshal(c.params, &got); err != nil {
				t.Fatalf("params %s: %v", c.params, err)
			}
			if len(got) != 2 || got["platformId"] != float64(3) || got["emulatorId"] != tc.want {
				t.Errorf("params = %s, want platformId 3 and emulatorId %v", c.params, tc.want)
			}
		})
	}
}
