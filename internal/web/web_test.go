package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestConsoleServedAtRoot(t *testing.T) {
	rec := get(t, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "DeviceDeck") {
		t.Fatalf("root: %d", rec.Code)
	}
}

func TestStaticAssets(t *testing.T) {
	for _, path := range []string{"/app.js", "/style.css", "/device.js", "/video-common.js", "/input-common.js"} {
		if rec := get(t, path); rec.Code != 200 {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

func TestDevicePage(t *testing.T) {
	rec := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951")
	if rec.Code != 200 {
		t.Fatalf("device page: %d", rec.Code)
	}
	body := rec.Body.String()
	// The automation page: mirror container present, and both scripts
	// wired — device.js depends on input-common.js for the frame
	// encoders and the queueing input socket, so a missing tag breaks
	// input silently rather than loudly.
	for _, want := range []string{`id="mirror"`, "device.js", "input-common.js", `id="video"`} {
		if !strings.Contains(body, want) {
			t.Errorf("device page missing %q", want)
		}
	}
}

// The console page loads the same shared modules; a missing tag there
// costs the console its input path while the video still renders, which
// looks like a device fault rather than a page fault.
func TestConsoleWiresSharedModules(t *testing.T) {
	body := get(t, "/").Body.String()
	for _, want := range []string{"video-common.js", "input-common.js", "app.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("console missing %q", want)
		}
	}
}

// The console header must be able to wrap. Wider than the window, it is
// not merely clipped: focusing a control near its right edge scrolls the
// page sideways, and the device and its inspector overlay slide
// off-centre together, which reads as broken scaling rather than as a
// header that did not fit.
func TestConsoleHeaderWraps(t *testing.T) {
	css := get(t, "/style.css").Body.String()
	header := css[strings.Index(css, "header {"):]
	header = header[:strings.Index(header, "}")]
	if !strings.Contains(header, "flex-wrap: wrap") {
		t.Errorf("header must wrap rather than overflow:\n%s", header)
	}
	if !strings.Contains(css, "input#app") || !strings.Contains(css, "min-width: 110px") {
		t.Error("the app-id input must be allowed to shrink")
	}
}
