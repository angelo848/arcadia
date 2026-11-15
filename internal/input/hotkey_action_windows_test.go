//go:build windows

package input

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestHotkeyAction(t *testing.T) {
	t.Run("CON-003/RegisterHotKey só para Ctrl+Alt+P e o gatilho vem com ActionPause", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) })
		if DefaultHotkey.VK != 0x50 || DefaultHotkey.Modifiers != ModControl|ModAlt {
			t.Errorf("DefaultHotkey = %+v, want Ctrl+Alt+P only", DefaultHotkey)
		}
		clock := domain.NewFakeClock(hotkeyStart)
		hk, out, _ := startHotkey(t, clock, 4)
		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey)
		tr := recvTrigger(t, out)
		if tr.Action != ActionPause || tr.Source != domain.PauseHotkey {
			t.Errorf("trigger = %+v, want {hotkey, ActionPause}", tr)
		}
	})
}
