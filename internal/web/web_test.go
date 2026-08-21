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

// The device page is a test target, so it must contain nothing but the
// device. Any control of ours living there would appear in an agent's
// snapshot beside the app's own elements — indistinguishable from them —
// and an agent told to "tap the button" could drive DeviceDeck instead
// of the app under test. The console is where our controls belong.
func TestDevicePageCarriesNoControlsOfOurOwn(t *testing.T) {
	body := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951").Body.String()
	// Everything between <body> and the scripts is the test target.
	start := strings.Index(body, "<body>")
	end := strings.Index(body, "<script")
	if start < 0 || end < 0 {
		t.Fatal("device page shape changed; this guard needs updating")
	}
	markup := body[start:end]
	for _, forbidden := range []string{"<button", "<input", "<select", "<textarea", "<a "} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("device page carries %q — an agent would see it as part of the app:\n%s",
				forbidden, markup)
		}
	}
	// The mirror and the video are the only things that belong.
	for _, want := range []string{`id="video"`, `id="mirror"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("device page is missing %s", want)
		}
	}
}

// The refusal notice is the one piece of our own wording on the automation
// page, so it has to be provably invisible to a locator. Two things make
// that true, and both were needed: aria-hidden keeps it out of getByRole
// and the ARIA snapshot, and keeping the element empty keeps it out of
// getByText — which was measured still matching an aria-hidden node. A
// refused page must never offer an agent a sentence the app never had.
func TestDeviceNoticeIsHiddenFromAutomation(t *testing.T) {
	body := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951").Body.String()
	start := strings.Index(body, "<body>")
	end := strings.Index(body, "<script")
	if start < 0 || end < 0 {
		t.Fatal("device page shape changed; this guard needs updating")
	}
	markup := body[start:end]
	notice := strings.Index(markup, `id="notice"`)
	if notice < 0 {
		t.Fatal("device page has no refusal notice; a refused page would look merely broken")
	}
	// The attribute must sit on the notice element itself.
	tagStart := strings.LastIndex(markup[:notice], "<")
	tagEnd := strings.Index(markup[notice:], ">") + notice
	if tag := markup[tagStart : tagEnd+1]; !strings.Contains(tag, `aria-hidden="true"`) {
		t.Errorf("notice is not aria-hidden, so automation would read it as app content: %s", tag)
	}
	// It must also live outside the mirror, which is the app's subtree.
	if mirror := strings.Index(markup, `id="mirror"`); mirror > notice {
		t.Error("notice precedes the mirror; it must not sit inside the app subtree")
	}
	// Empty element: the wording is painted by CSS from an attribute, so
	// there is no text node for getByText to match.
	if body := markup[tagEnd+1:]; !strings.HasPrefix(strings.TrimSpace(body), "</div>") {
		t.Errorf("notice carries text in the DOM, which getByText would match: %.60s", body)
	}
}

// The mirror is invisible to the eye but must stay visible to every
// machine that reads the page. That rests on one detail: the nodes are
// unpainted, not hidden. color:transparent leaves a real box in layout,
// so Chrome exposes the element and Playwright can click it — measured at
// 160 exposed nodes with none ignored. Switching to opacity:0 or
// visibility:hidden would look identical on screen and silently empty the
// accessibility tree, which is the one failure that would make DeviceDeck
// useless while appearing to work.
func TestMirrorHidesFromTheEyeAndNotFromTheMachine(t *testing.T) {
	body := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951").Body.String()
	rule := "#mirror [data-dd-node] {"
	start := strings.Index(body, rule)
	if start < 0 {
		t.Fatal("mirror node rule not found; this guard needs updating")
	}
	end := strings.Index(body[start:], "}") + start
	css := body[start:end]
	for _, banned := range []string{"opacity: 0", "opacity:0", "visibility: hidden", "visibility:hidden"} {
		if strings.Contains(css, banned) {
			t.Errorf("mirror nodes use %q, which removes them from the accessibility tree: %s",
				banned, css)
		}
	}
	if !strings.Contains(css, "color: transparent") {
		t.Error("mirror nodes no longer rely on color:transparent; confirm they are still exposed to automation")
	}
}
