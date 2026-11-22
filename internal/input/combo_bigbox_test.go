package input

import "testing"

func TestBigBoxCombo(t *testing.T) {
	t.Parallel()

	t.Run("ATB-001/defaultBigBoxMask é Back|A (0x1020) e ButtonA é 0x1000", func(t *testing.T) {
		t.Parallel()
		const want uint16 = 0x1020
		if uint16(defaultBigBoxMask) != want {
			t.Fatalf("defaultBigBoxMask = %#x, want %#x", uint16(defaultBigBoxMask), want)
		}
		if uint16(ButtonBack)|uint16(ButtonA) != want || uint16(ButtonA) != 0x1000 {
			t.Fatalf("ButtonA = %#x, Back|A = %#x", uint16(ButtonA), uint16(ButtonBack)|uint16(ButtonA))
		}
	})

	t.Run("ATB-002/ActionBigBox é distinta de ActionPause e ActionClose", func(t *testing.T) {
		t.Parallel()
		if ActionBigBox == ActionPause || ActionBigBox == ActionClose {
			t.Fatalf("ActionBigBox = %d collides with pause/close", ActionBigBox)
		}
	})
}
