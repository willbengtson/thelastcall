// Package webui embeds the built frontend into the server binary, so a deploy
// is one artifact.
package webui

import (
	"embed"
	"io/fs"
)

// dist holds the Vite build output. The .gitkeep keeps the embed valid before
// the frontend has ever been built.
//
//go:embed all:dist
var dist embed.FS

// FS returns the built frontend rooted at its index.html, or nil if the
// frontend has not been built into this binary. A nil FS makes the server
// API-only, which is what you want while running the Vite dev server.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
