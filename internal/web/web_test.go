package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return getWith(t, path, nil)
}

func getWith(t *testing.T, path string, first FirstTree) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(first).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
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
	// The app field and the copyable links live in the sidebar, whose
	// inputs must shrink to its width rather than push it wider.
	sidebar := css[strings.Index(css, "#sidebar input, #sidebar select {"):]
	if !strings.Contains(sidebar[:strings.Index(sidebar, "}")], "min-width: 0") {
		t.Error("sidebar inputs must be allowed to shrink")
	}
}

// The sidebar controls keep the element ids the console's tests and
// scripts select them by, wherever the layout puts them.
func TestConsoleKeepsControlIDs(t *testing.T) {
	body := get(t, "/").Body.String()
	for _, id := range []string{
		"app", "app-pick", "btn-launch", "btn-home", "btn-switcher", "btn-lock",
		"btn-shot", "btn-inspect", "btn-record", "btn-assert", "device-label", "status",
	} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("console lost #%s", id)
		}
	}
}

// The device page is a test target, so it must contain nothing but the
// device. Any control of ours living there would appear in an agent's
// snapshot beside the app's own elements — indistinguishable from them —
// and an agent told to "tap the button" could drive DeviceDeck instead
// of the app under test. The console is where our controls belong.
// This checks the page as served. The mirror does create <input> elements
// at runtime, one per native text field, and those are the point — they
// are the app's controls, not ours. What must never appear is a control
// authored here: an agent cannot tell our button from the app's, and
// "tap the button" would drive DeviceDeck instead of the app under test.
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

// Mirrored text fields are <input> elements, and unlike a div a user agent
// paints those: a border, an opaque background, a caret, and a placeholder
// that keeps its own colour whatever `color` says. Shipped without the
// reset, they drew two white boxes over the video and a second copy of each
// field's prompt on top of the app's own — the mirror is meant to add no
// pixels at all. Every one of these was a separate leak, so each is named.
func TestMirroredFieldsPaintNothing(t *testing.T) {
	body := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951").Body.String()
	for _, required := range []string{
		"appearance: none",
		"border: 0",
		"background: transparent",
		"caret-color: transparent",
		"::placeholder { color: transparent; }",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("mirrored inputs are missing %q, so they will paint over the video", required)
		}
	}
}

// Container nodes are click-through so they stop swallowing clicks aimed
// at the leaves inside them — but a control that happens to contain
// something else is still a control. Without the exception the cart
// button became unclickable the moment it grew a badge, and an agent
// could only reach it through the child image. Only tappable roles are
// exempt: a list or a navigation bar spans its children and would go
// straight back to swallowing them.
func TestControlsStayClickableEvenWithChildren(t *testing.T) {
	body := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951").Body.String()
	if !strings.Contains(body, `:not(:has([data-dd-node])) { pointer-events: auto; }`) {
		t.Error("leaf nodes are no longer clickable; every click would hit a container")
	}
	for _, role := range []string{"button", "link", "textbox", "searchbox"} {
		want := `[role="` + role + `"]`
		if !strings.Contains(body, want) {
			t.Errorf("role %q is not exempted from click-through, so such a control "+
				"becomes unclickable as soon as it contains anything", role)
		}
	}
	// The roles that must NOT be exempt, because they span their children.
	for _, role := range []string{"list", "navigation", "listitem"} {
		if strings.Contains(body, `[role="`+role+`"] { pointer-events: auto`) {
			t.Errorf("role %q is exempted; it spans its children and will swallow their clicks", role)
		}
	}
}

// The device page carries its first tree, so the mirror is complete
// when load fires — the one event an agent's navigate waits for.
func TestDevicePageInlinesFirstTree(t *testing.T) {
	var gotUDID, gotApp string
	first := func(ctx context.Context, udid, app string) ([]byte, error) {
		gotUDID, gotApp = udid, app
		return []byte(`{"nodes":[],"hash":"abc"}`), nil
	}
	body := getWith(t, "/device/AAA?app=com.example", first).Body.String()
	if !strings.Contains(body, `<script id="dd-first-tree" type="application/json">{"nodes":[],"hash":"abc"}</script>`) {
		t.Errorf("first tree not inlined: %s", body)
	}
	if gotUDID != "AAA" || gotApp != "com.example" {
		t.Errorf("first tree asked for %q/%q", gotUDID, gotApp)
	}
	if ct := getWith(t, "/device/AAA", first).Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type %q", ct)
	}
}

// A label can carry the one sequence that ends a script element early.
func TestDevicePageEscapesTreeForScriptSlot(t *testing.T) {
	first := func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"nodes":[{"label":"</script><b>x"}]}`), nil
	}
	body := getWith(t, "/device/AAA", first).Body.String()
	if strings.Contains(body, `"label":"</script>`) {
		t.Errorf("raw </script> inside the slot: %s", body)
	}
	if !strings.Contains(body, `<\/script><b>x`) {
		t.Errorf("slash not escaped: %s", body)
	}
}

// No tree, no slot filled: the page fetches as it always did.
func TestDevicePageWithoutFirstTree(t *testing.T) {
	failing := func(context.Context, string, string) ([]byte, error) { return nil, errors.New("engine down") }
	for name, first := range map[string]FirstTree{"nil": nil, "failing": failing} {
		body := getWith(t, "/device/AAA", first).Body.String()
		if !strings.Contains(body, firstTreeSlot) {
			t.Errorf("%s: empty slot expected in page: %s", name, body)
		}
	}
}

func TestDevicePageWithAPlaceholderIDGoesHome(t *testing.T) {
	for _, path := range []string{"/device/%3Cudid%3E", "/device/%7Budid%7D"} {
		rec := get(t, path)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
			t.Errorf("%s: %d to %q, want a redirect to /", path, rec.Code, rec.Header().Get("Location"))
		}
	}
}
