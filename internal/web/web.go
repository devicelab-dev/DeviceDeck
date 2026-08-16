// Package web serves the embedded browser console: video canvas, live
// input, and the tree inspector overlay.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// Handler serves the console's static assets with index.html at /.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Unreachable: the embed directive guarantees the directory.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
