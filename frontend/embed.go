// Package frontend embeds the built web assets (frontend/dist) served by the Wails UI.
package frontend

import "embed"

// Assets holds the contents of frontend/dist, including dotfiles such as .gitkeep,
// so the module builds even before the frontend has been compiled with Node.
//
//go:embed all:dist
var Assets embed.FS
