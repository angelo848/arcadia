//go:build windows

package agent

import (
	"io"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// realPlatform instantiates the real Windows sources, power watcher and feedback components [T20].
func realPlatform(clock domain.Clock, log *slog.Logger) platformParts {
	var parts platformParts

	// 1. Keyboard hotkey (Ctrl+Alt+P)
	parts.Sources = append(parts.Sources, input.NewHotkey(input.DefaultHotkey, clock, log))

	// 2. Controller poller via XInput (Select + Y pause, Select + B close, Select + D-pad up/down volume)
	padAPI, err := input.NewXInput()
	if err != nil {
		log.Warn("controle indisponível (XInput)", "error", err.Error())
	} else {
		parts.Sources = append(parts.Sources, input.NewPad(padAPI, input.PadConfig{Volume: windowsVolume(log)}, clock, log))
	}

	// 3. Power watcher (WM_POWERBROADCAST)
	parts.Power = func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error) {
		pw := winapi.NewPowerWatcher(clock, log, winapi.PowerCallbacks{
			OnSuspend: onSuspend,
			OnWake:    onWake,
		})
		if err := pw.Start(); err != nil {
			return nil, err
		}
		return pw, nil
	}

	// 4. Feedback overlay and sound
	parts.NewOverlay = func() (feedback.Overlay, error) {
		ov := winapi.NewOverlay(log)
		if err := ov.Start(); err != nil {
			return nil, err
		}
		return ov, nil
	}

	parts.NewSound = func() (feedback.Sound, error) {
		return winapi.NewBeeper(log), nil
	}

	return parts
}

// windowsVolume returns the callback of the gamepad volume chords: one Windows volume key per step.
func windowsVolume(log *slog.Logger) func(up bool) {
	return func(up bool) {
		if err := winapi.VolumeStep(up); err != nil {
			log.Warn("falha ao mudar o volume", "up", up, "error", err.Error())
		}
	}
}
