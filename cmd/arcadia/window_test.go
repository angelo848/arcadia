package main

// Tests for the main window options (UIX-001, UIX-002). No build tag: applyWindowOptions
// is declarative and lives in window.go.

import (
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestApplyWindowOptions(t *testing.T) {
	t.Parallel()

	t.Run("UIX-001/WindowStartState é Maximised e Fullscreen é falso", func(t *testing.T) {
		t.Parallel()
		var o options.App
		applyWindowOptions(&o)
		if o.WindowStartState != options.Maximised {
			t.Errorf("WindowStartState = %v, want options.Maximised", o.WindowStartState)
		}
		if o.Fullscreen { //nolint:staticcheck // also guard the legacy flag
			t.Error("Fullscreen = true, want false")
		}
		if o.WindowStartState == options.Fullscreen {
			t.Error("WindowStartState = Fullscreen, want not fullscreen")
		}
	})

	t.Run("UIX-002/Width 1280, Height 800, MinWidth 1024, MinHeight 640", func(t *testing.T) {
		t.Parallel()
		var o options.App
		applyWindowOptions(&o)
		if o.Width != 1280 || o.Height != 800 {
			t.Errorf("size = %dx%d, want 1280x800", o.Width, o.Height)
		}
		if o.MinWidth != 1024 || o.MinHeight != 640 {
			t.Errorf("min size = %dx%d, want 1024x640", o.MinWidth, o.MinHeight)
		}
	})
}
