//go:build windows

package input

import (
	"errors"
	"testing"
)

func TestXInput(t *testing.T) {
	t.Run("PAD-007/DLL inexistente devolve erro e nenhum PadAPI", func(t *testing.T) {
		api, err := loadXInput("arcadia-no-such-xinput-9.dll")
		if err == nil {
			t.Fatal("loadXInput with a missing DLL returned nil error")
		}
		if api != nil {
			t.Fatalf("loadXInput with a missing DLL returned a non-nil PadAPI: %v", api)
		}
	})

	t.Run("PAD-007/NewXInput real carrega e State(0) devolve estado ou ErrDisconnected, sem pânico", func(t *testing.T) {
		api, err := NewXInput()
		if err != nil {
			t.Fatalf("NewXInput: %v", err)
		}
		if api == nil {
			t.Fatal("NewXInput returned nil PadAPI without error")
		}
		_, err = api.State(0)
		if err != nil && !errors.Is(err, ErrDisconnected) {
			t.Fatalf("State(0) error = %v, want nil or wrapping ErrDisconnected", err)
		}
	})

	t.Run("PAD-007/State com slot fora de 0..3 devolve erro", func(t *testing.T) {
		api, err := NewXInput()
		if err != nil {
			t.Fatalf("NewXInput: %v", err)
		}
		for _, slot := range []int{-1, 4, 100} {
			if _, err := api.State(slot); err == nil {
				t.Errorf("State(%d) returned nil error", slot)
			}
		}
	})
}
