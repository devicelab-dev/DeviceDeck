// Package web serves the embedded browser console: video canvas, live
// input, the tree inspector overlay, and the per-device automation page.
package web

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

//go:embed static
var staticFS embed.FS

// FirstTree supplies the settled tree for a device page, as the JSON the
// page's tree endpoint would return. It is called while the page's HTML
// is being served, so it should return once the screen is quiet.
type FirstTree func(ctx context.Context, udid, app string) ([]byte, error)

// firstTreeSlot is where the page carries its first tree. The page
// renders from it synchronously, before the load event.
const firstTreeSlot = `<script id="dd-first-tree" type="application/json"></script>`

// Handler serves the console's static assets with index.html at /, plus
// the automation page at /device/{udid} — the "device as a normal
// webpage" surface that Playwright/Cypress/Selenium drive.
//
// The device page is served with its first tree inlined, because an
// agent's navigate waits for the load event and nothing else: a tree
// fetched after load is a tree the agent never saw, and its first
// snapshot was an empty mirror with one ref on a page whose app was
// fully up. With the tree in the HTML the page is complete when load
// fires, the same way a server-rendered page is. first may be nil, and
// a first tree that fails is logged and left out — the page fetches as
// it always did, and the only cost is that first snapshot.
//
// Coverage waiver: the two panics guard fs.Sub/fs.ReadFile against a
// missing embed. The //go:embed directive makes both failures impossible
// to produce at runtime, so the arms are unreachable and left uncovered.
func Handler(first FirstTree) http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Unreachable: the embed directive guarantees the directory.
		panic(err)
	}
	page, err := fs.ReadFile(sub, "device.html")
	if err != nil {
		panic(err) // Same guarantee: the page is embedded.
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /device/{udid}", func(w http.ResponseWriter, r *http.Request) {
		// An id no device can have, typically the startup guide's
		// "<udid>" placeholder clicked as is, goes to the console, where
		// the real devices are listed.
		if !platform.ValidID(r.PathValue("udid")) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(withFirstTree(r, page, first))
	})
	return mux
}

// withFirstTree fills the page's tree slot, or leaves it empty.
func withFirstTree(r *http.Request, page []byte, first FirstTree) []byte {
	if first == nil {
		return page
	}
	udid, app := r.PathValue("udid"), r.URL.Query().Get("app")
	tree, err := first(r.Context(), udid, app)
	if err != nil {
		slog.Warn("first tree for device page failed", "udid", udid, "err", err)
		return page
	}
	return bytes.Replace(page, []byte(firstTreeSlot), inlineJSON(tree), 1)
}

// inlineJSON wraps JSON for a script slot. The one sequence that can end
// the element early is "</", which a label could carry; escaping the
// slash keeps it JSON and keeps it inside the tag.
func inlineJSON(tree []byte) []byte {
	safe := bytes.ReplaceAll(tree, []byte("</"), []byte(`<\/`))
	return append(append([]byte(`<script id="dd-first-tree" type="application/json">`), safe...), []byte("</script>")...)
}
