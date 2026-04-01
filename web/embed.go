// Package web holds the embedded static web UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var files embed.FS

// FS exposes only the built UI files (web/dist/ contents).
var FS, _ = fs.Sub(files, "dist")
