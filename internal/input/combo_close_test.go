package input

import "testing"

func TestCloseCombo(t *testing.T) {
	t.Parallel()

	t.Run("CON-001/defaultCloseMask é Back|B (0x2020)", func(t *testing.T) {
		t.Parallel()
		const want uint16 = 0x2020
		if uint16(defaultCloseMask) != want {
			t.Fatalf("defaultCloseMask = %#x, want %#x", defaultCloseMask, want)
		}
		if uint16(ButtonBack)|uint16(ButtonB) != want {
			t.Fatalf("ButtonBack|ButtonB = %#x, want %#x", uint16(ButtonBack)|uint16(ButtonB), want)
		}
	})
}
