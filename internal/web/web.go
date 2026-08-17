// Package web serves the embedded browser console: video canvas, live
// input, the tree inspector overlay, and the per-device automation page.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// Handler serves the console's static assets with index.html at /, plus
// the automation page at /device/{udid} — the "device as a normal
// webpage" surface that Playwright/Cypress/Selenium drive.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Unreachable: the embed directive guarantees the directory.
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /device/{udid}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, sub, "device.html")
	})
	return mux
}
