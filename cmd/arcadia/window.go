package main

import "github.com/wailsapp/wails/v2/pkg/options"

// applyWindowOptions sets the main window geometry: maximised (not fullscreen)
// at start, with a 1280x800 restored size and a 1024x640 minimum (UIX-001, UIX-002).
func applyWindowOptions(o *options.App) {
	o.WindowStartState = options.Maximised
	o.Fullscreen = false
	o.Width = 1280
	o.Height = 800
	o.MinWidth = 1024
	o.MinHeight = 640
}
