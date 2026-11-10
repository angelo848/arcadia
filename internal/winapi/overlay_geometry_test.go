package winapi

import "testing"

func TestOverlayRect(t *testing.T) {
	t.Parallel()

	t.Run("OVL-004/área (0,0,1920,1040) 240x56 a 96 dpi: canto (840,48)", func(t *testing.T) {
		t.Parallel()
		w, h := overlaySize(96)
		got := overlayRect(Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, 96, w, h)
		want := Rect{Left: 840, Top: 48, Right: 840 + 240, Bottom: 48 + 56}
		if got != want {
			t.Fatalf("overlayRect = %+v, want %+v", got, want)
		}
	})

	t.Run("OVL-004/área (1920,0,3840,1040): canto (2760,48)", func(t *testing.T) {
		t.Parallel()
		w, h := overlaySize(96)
		got := overlayRect(Rect{Left: 1920, Top: 0, Right: 3840, Bottom: 1040}, 96, w, h)
		want := Rect{Left: 2760, Top: 48, Right: 2760 + 240, Bottom: 48 + 56}
		if got != want {
			t.Fatalf("overlayRect = %+v, want %+v", got, want)
		}
	})

	t.Run("OVL-004/144 dpi 360x84 na primeira área: canto (780,72)", func(t *testing.T) {
		t.Parallel()
		w, h := overlaySize(144)
		if w != 360 || h != 84 {
			t.Fatalf("overlaySize(144) = %dx%d, want 360x84", w, h)
		}
		got := overlayRect(Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, 144, w, h)
		want := Rect{Left: 780, Top: 72, Right: 780 + 360, Bottom: 72 + 84}
		if got != want {
			t.Fatalf("overlayRect = %+v, want %+v", got, want)
		}
	})

	t.Run("OVL-004/overlaySize escala por dpi/96", func(t *testing.T) {
		t.Parallel()
		cases := []struct{ dpi, w, h int }{
			{96, 240, 56},
			{144, 360, 84},
			{192, 480, 112},
		}
		for _, c := range cases {
			w, h := overlaySize(c.dpi)
			if w != c.w || h != c.h {
				t.Errorf("overlaySize(%d) = %dx%d, want %dx%d", c.dpi, w, h, c.w, c.h)
			}
		}
		if got := (Rect{Left: 1920, Right: 3840}).Width(); got != 1920 {
			t.Errorf("Rect.Width = %d, want 1920", got)
		}
	})
}
