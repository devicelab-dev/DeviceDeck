package web

import (
	"embed"
	"testing"
)

// TestHandlerPanicsWithoutThePage covers the guard for a build that lost
// the embedded device page: the handler refuses to start rather than
// serve a console with no automation page.
func TestHandlerPanicsWithoutThePage(t *testing.T) {
	saved := staticFS
	staticFS = embed.FS{}
	defer func() {
		staticFS = saved
		if recover() == nil {
			t.Fatal("Handler must panic when device.html is not embedded")
		}
	}()
	Handler(nil)
}
