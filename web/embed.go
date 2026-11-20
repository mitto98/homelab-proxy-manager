// Package web embeds the built single-page app.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built SPA rooted at its index.html.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time, so this cannot fail
	}
	return sub
}
