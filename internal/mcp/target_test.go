package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// routedAPI answers by path — the tree for GET /tree, a scripted reply for
// each POST — and records every call, for tools that make several.
type routedAPI struct {
	srv   *httptest.Server
	mu    sync.Mutex
	tree  string
	posts map[string]string // path suffix -> body; missing = {"ok":true}
	fail  map[string]bool   // path suffix -> answer 502
	calls []string          // "METHOD path body"
}

func newRoutedAPI(tree string) *routedAPI {
	r := &routedAPI{tree: tree, posts: map[string]string{}, fail: map[string]bool{}}
	r.srv = httptest.NewServer(http.HandlerFunc(r.serve))
	return r
}

func (r *routedAPI) serve(w http.ResponseWriter, req *http.Request) {
	b, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req.Method+" "+req.URL.Path+" "+string(b))
	suffix := req.URL.Path[strings.LastIndex(req.URL.Path, "/"):]
	if r.fail[suffix] {
		w.WriteHeader(502)
		return
	}
	if req.Method == http.MethodGet {
		_, _ = io.WriteString(w, r.tree)
		return
	}
	body, ok := r.posts[suffix]
	if !ok {
		body = `{"ok":true}`
	}
	_, _ = io.WriteString(w, body)
}

func (r *routedAPI) client() *Client { return NewClient(r.srv.URL) }

// last returns the most recent call whose path ends in suffix.
func (r *routedAPI) last(suffix string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.calls) - 1; i >= 0; i-- {
		if strings.Contains(r.calls[i], suffix+" ") {
			return r.calls[i]
		}
	}
	return ""
}

// formTree is a login screen: two fields (one with an id, one without), two
// "Add" buttons without ids, and a Sign In button.
func formTree() string {
	return tree(
		`{"type":"TextField","identifier":"username","placeholder":"Username","value":"old","frame":{"x":0,"y":100,"width":200,"height":40}}`,
		`{"type":"SecureTextField","placeholder":"Password","frame":{"x":0,"y":200,"width":200,"height":40}}`,
		`{"type":"Button","label":"Add","frame":{"x":0,"y":300,"width":100,"height":40}}`,
		`{"type":"Button","label":"Add","frame":{"x":100,"y":300,"width":100,"height":40}}`,
		`{"type":"Button","label":"Sign In","frame":{"x":0,"y":400,"width":200,"height":40}}`)
}

// A ref read from snapshot taps the element it named; a ref for an element
// that has left the screen, or one never issued, fails.
func TestTapByRef(t *testing.T) {
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	snap, err := c.snapshot(raw(map[string]any{"udid": "U"}))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ref := refFor(t, snap, `"Sign In"`)
	if out, err := c.tap(raw(map[string]any{"udid": "U", "ref": ref})); err != nil || out != "tapped "+ref {
		t.Fatalf("tap by ref = %q, %v", out, err)
	}
	if got := api.last("/tap"); !strings.Contains(got, `"x":0.25,"y":0.525`) { // Sign In: centre (100,420) on 400x800
		t.Errorf("tapped the wrong place: %s", got)
	}
	api.tree = tree(`{"type":"Button","label":"Other","frame":{"x":0,"y":0,"width":10,"height":10}}`)
	if _, err := c.tap(raw(map[string]any{"udid": "U", "ref": ref})); err == nil || !strings.Contains(err.Error(), "not on the current screen") {
		t.Errorf("stale ref: %v", err)
	}
	for _, bad := range []string{"e999", "x1", "e01", "e0"} {
		if _, err := c.tap(raw(map[string]any{"udid": "U", "ref": bad})); err == nil || !strings.Contains(err.Error(), "unknown ref") {
			t.Errorf("ref %q: %v", bad, err)
		}
	}
}

// refFor pulls the ref of the snapshot line containing name.
func refFor(t *testing.T, snap, name string) string {
	t.Helper()
	for _, line := range strings.Split(snap, "\n") {
		if strings.Contains(line, name) {
			return strings.Fields(strings.TrimPrefix(line, "* "))[0]
		}
	}
	t.Fatalf("no %s in snapshot:\n%s", name, snap)
	return ""
}

// Text and role/name must name exactly one element.
func TestTargetIndexSelectors(t *testing.T) {
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"text by placeholder", map[string]any{"text": "password"}, ""},
		{"text by label", map[string]any{"text": "sign in"}, ""},
		{"text ambiguous", map[string]any{"text": "add"}, "2 elements match"},
		{"text absent", map[string]any{"text": "nope"}, "no element with text"},
		{"role and name", map[string]any{"role": "button", "name": "sign"}, ""},
		{"role alone ambiguous", map[string]any{"role": "button"}, "3 elements match"},
		{"testid", map[string]any{"testid": "username"}, ""},
		{"testid absent", map[string]any{"testid": "gone"}, "no element with testid"},
		{"nothing named", map[string]any{}, "name the element"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args["udid"] = "U"
			_, err := c.tap(raw(tt.args))
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestTargetName(t *testing.T) {
	for _, tt := range []struct {
		a    deviceArgs
		want string
	}{
		{deviceArgs{Ref: "e3", Testid: "x"}, "e3"},
		{deviceArgs{Testid: "x", Text: "y"}, "x"},
		{deviceArgs{Text: "y"}, "y"},
		{deviceArgs{Role: "button", Name: "Go"}, "button Go"},
		{deviceArgs{Name: "Go"}, "Go"},
	} {
		if got := targetName(tt.a); got != tt.want {
			t.Errorf("targetName(%+v) = %q, want %q", tt.a, got, tt.want)
		}
	}
}

// fill posts one driver fill through /act: the field's id when unique, its
// centre in both units, the value, and how much it held.
func TestFill(t *testing.T) {
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	out, err := c.fill(raw(map[string]any{"udid": "U", "app": "com.x", "testid": "username", "value": "devicelab"}))
	if err != nil || out != "filled username" {
		t.Fatalf("fill = %q, %v", out, err)
	}
	got := api.last("/act")
	for _, want := range []string{`"kind":"fill"`, `"field":"username"`, `"text":"devicelab"`, `"prev":3`, `"px":100`, `"py":120`, `"app":"com.x"`, `"id":"mcp-`} {
		if !strings.Contains(got, want) {
			t.Errorf("act body missing %s: %s", want, got)
		}
	}
	// A field without an id is addressed by its centre alone; submit presses Enter.
	out, err = c.fill(raw(map[string]any{"udid": "U", "text": "password", "value": "pw", "submit": true}))
	if err != nil || out != "filled password and pressed enter" {
		t.Fatalf("fill+submit = %q, %v", out, err)
	}
	if strings.Contains(api.last("/act"), `"field"`) || !strings.Contains(api.last("/key"), `"usage":40`) {
		t.Errorf("act %s / key %s", api.last("/act"), api.last("/key"))
	}
}

func TestFillFailures(t *testing.T) {
	disabled := tree(`{"type":"TextField","identifier":"f","enabled":false,"frame":{"x":0,"y":0,"width":10,"height":10}}`)
	tests := []struct {
		name    string
		tree    string
		posts   map[string]string
		fail    string
		args    map[string]any
		wantErr string
	}{
		{"no udid", formTree(), nil, "", map[string]any{"testid": "username"}, "udid is required"},
		{"no target", formTree(), nil, "", map[string]any{"udid": "U"}, "name the element"},
		{"disabled", disabled, nil, "", map[string]any{"udid": "U", "testid": "f"}, "disabled"},
		{"no screen size", `{"nodes":[{"type":"Application","frame":{"width":0,"height":0}},{"type":"TextField","identifier":"f","frame":{"x":0,"y":0,"width":10,"height":10}}]}`, nil, "", map[string]any{"udid": "U", "testid": "f"}, "no screen dimensions"},
		{"driver refuses", formTree(), map[string]string{"/act": `{"act":{"error":"device holds \"x\""}}`}, "", map[string]any{"udid": "U", "testid": "username"}, `device holds "x"`},
		{"bad act reply", formTree(), map[string]string{"/act": `nope`}, "", map[string]any{"udid": "U", "testid": "username"}, "decode action result"},
		{"act post fails", formTree(), nil, "/act", map[string]any{"udid": "U", "testid": "username"}, "fill username"},
		{"enter fails", formTree(), nil, "/key", map[string]any{"udid": "U", "testid": "username", "submit": true}, "502"},
		{"tree fails", formTree(), nil, "/tree", map[string]any{"udid": "U", "testid": "username"}, "502"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newRoutedAPI(tt.tree)
			defer api.srv.Close()
			for k, v := range tt.posts {
				api.posts[k] = v
			}
			if tt.fail != "" {
				api.fail[tt.fail] = true
			}
			_, err := api.client().fill(raw(tt.args))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewActionIDUnique(t *testing.T) {
	if a, b := newActionID(), newActionID(); a == b || !strings.HasPrefix(a, "mcp-") {
		t.Errorf("ids %q %q", a, b)
	}
}

// A target whose centre is off the screen or under the keyboard is refused
// with what to do instead; the keyboard's own keys are reachable.
func TestReachable(t *testing.T) {
	kbTree := `{"nodes":[
		{"index":0,"type":"Application","frame":{"x":0,"y":0,"width":400,"height":800}},
		{"index":1,"parentIndex":0,"type":"TextField","identifier":"low","frame":{"x":0,"y":700,"width":400,"height":40}},
		{"index":2,"parentIndex":0,"type":"Button","identifier":"gone","frame":{"x":0,"y":900,"width":100,"height":40}},
		{"index":3,"parentIndex":0,"type":"Keyboard","frame":{"x":0,"y":500,"width":400,"height":300}},
		{"index":4,"parentIndex":3,"type":"Key","identifier":"q","frame":{"x":0,"y":520,"width":40,"height":40}},
		{"index":5,"parentIndex":0,"type":"Button","identifier":"top","frame":{"x":0,"y":100,"width":100,"height":40}}]}`
	api := newRoutedAPI(kbTree)
	defer api.srv.Close()
	c := api.client()
	tests := []struct {
		testid, wantErr string
	}{
		{"gone", "off screen"},
		{"low", "under the on-screen keyboard"},
		{"q", ""},
		{"top", ""},
	}
	for _, tt := range tests {
		_, err := c.tap(raw(map[string]any{"udid": "U", "testid": tt.testid}))
		if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("tap %s: err = %v, want %q", tt.testid, err, tt.wantErr)
		}
	}
	if _, err := c.fill(raw(map[string]any{"udid": "U", "testid": "low", "value": "x"})); err == nil ||
		!strings.Contains(err.Error(), "keyboard") {
		t.Errorf("fill under the keyboard: %v", err)
	}
}

func TestDescendsStopsOnCycles(t *testing.T) {
	one, zero := 1, 0
	nodes := []treeNode{{Index: 0, ParentIndex: &one}, {Index: 1, ParentIndex: &zero}}
	if descends(nodes, nodes[0], treeNode{Index: 9}) {
		t.Error("a parent cycle must not count as descending")
	}
	if !descends(nodes, nodes[0], nodes[0]) {
		t.Error("a node descends from itself")
	}
}

// Android reports its status bar and keyboard as chrome, not nodes: a target
// under either is refused before anything is sent; one clear of both taps.
func TestTapRefusesAndroidChrome(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	body := func(extra string) string {
		return strings.Replace(tree(
			`{"index":1,"type":"Button","identifier":"cart-button","frame":{"x":800,"y":40,"width":100,"height":90}}`,
			`{"index":2,"type":"Button","identifier":"login-button","frame":{"x":64,"y":1601,"width":952,"height":66}}`,
			`{"index":3,"type":"Button","identifier":"search","frame":{"x":64,"y":400,"width":952,"height":66}}`),
			`{"nodes"`, `{`+extra+`"nodes"`, 1)
	}
	android := func(extra string) string {
		return strings.Replace(body(extra), `"width":400,"height":800`, `"width":1080,"height":2400`, 1)
	}
	f.body = android(`"chrome":{"statusBar":128,"keyboardTop":1517},`)
	for id, want := range map[string]string{"cart-button": "under the status bar", "login-button": "under the on-screen keyboard"} {
		f.lastPath = ""
		if _, err := c.tap(raw(map[string]any{"udid": "emulator-5554", "testid": id})); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", id, err, want)
		}
		if strings.HasSuffix(f.lastPath, "/tap") {
			t.Errorf("%s: a refused tap still reached the device", id)
		}
	}
	if _, err := c.tap(raw(map[string]any{"udid": "emulator-5554", "testid": "search"})); err != nil {
		t.Errorf("a target clear of the chrome was refused: %v", err)
	}
}
