// Package web holds the embedded static web UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed *.html *.js *.css
var files embed.FS

// FS exposes only the web files (without the embed.go itself).
var FS, _ = fs.Sub(files, ".")
