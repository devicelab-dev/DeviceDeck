package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAPI stands in for a running `devicedeck serve`, recording the last
// request and returning canned bodies keyed by path prefix.
type fakeAPI struct {
	srv        *httptest.Server
	lastMethod string
	lastPath   string
	lastQuery  string
	lastBody   string
	status     int
	body       string
	failPost   bool
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{status: 200, body: `{"ok":true}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.lastMethod, f.lastPath, f.lastQuery, f.lastBody = r.Method, r.URL.Path, r.URL.RawQuery, string(b)
		if f.failPost && r.Method == "POST" {
			w.WriteHeader(502)
			return
		}
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.body)
	}))
	return f
}

func (f *fakeAPI) client() *Client { return NewClient(f.srv.URL) }
func (f *fakeAPI) close()          { f.srv.Close() }

func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func TestNewClientTrimsSlash(t *testing.T) {
	if c := NewClient("http://x:1/"); c.BaseURL != "http://x:1" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
}

func TestToolsRegistered(t *testing.T) {
	tools, order := NewClient("http://x").Tools()
	if len(tools) != 16 || len(order) != 16 {
		t.Fatalf("want 16 tools, got %d/%d", len(tools), len(order))
	}
	for _, name := range order {
		if _, ok := tools[name]; !ok {
			t.Errorf("order names %q but it is not registered", name)
		}
	}
}

func TestDecodeArgs(t *testing.T) {
	if a, err := decodeArgs(nil); err != nil || a.UDID != "" {
		t.Errorf("empty args: %+v %v", a, err)
	}
	if a, err := decodeArgs(raw(map[string]string{"udid": "u1"})); err != nil || a.UDID != "u1" {
		t.Errorf("valid args: %+v %v", a, err)
	}
	if _, err := decodeArgs(json.RawMessage(`["bad"]`)); err == nil {
		t.Error("want error on non-object args")
	}
}

func TestListDevices(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = `{"devices":[{"udid":"u1"}]}`
	got, err := f.client().listDevices(nil)
	if err != nil || !strings.Contains(got, "u1") {
		t.Fatalf("listDevices = %q, %v", got, err)
	}
	if f.lastMethod != "GET" || f.lastPath != "/api/devices" {
		t.Errorf("called %s %s", f.lastMethod, f.lastPath)
	}
}

func TestDevicePageURL(t *testing.T) {
	c := NewClient("http://host:8787")
	got, err := c.devicePageURL(raw(map[string]string{"udid": "A B", "app": "com.x"}))
	if err != nil || got != "http://host:8787/device/A%20B?app=com.x" {
		t.Errorf("with app = %q, %v", got, err)
	}
	got, _ = c.devicePageURL(raw(map[string]string{"udid": "u1"}))
	if got != "http://host:8787/device/u1" {
		t.Errorf("without app = %q", got)
	}
	if _, err := c.devicePageURL(raw(map[string]string{})); err == nil {
		t.Error("want error when udid missing")
	}
}

func TestUITree(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = `{"nodes":[]}`
	if _, err := f.client().uiTree(raw(map[string]string{"udid": "u1", "app": "com.x"})); err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/devices/u1/tree" || f.lastQuery != "app=com.x" {
		t.Errorf("called %s?%s", f.lastPath, f.lastQuery)
	}
	// Without app: no query.
	_, _ = f.client().uiTree(raw(map[string]string{"udid": "u1"}))
	if f.lastQuery != "" {
		t.Errorf("unexpected query %q", f.lastQuery)
	}
	if _, err := f.client().uiTree(raw(map[string]string{})); err == nil {
		t.Error("want error when udid missing")
	}
}

func TestBootDevice(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	got, err := f.client().bootDevice(raw(map[string]string{"udid": "u1"}))
	if err != nil || !strings.Contains(got, "u1") {
		t.Fatalf("boot = %q, %v", got, err)
	}
	if f.lastMethod != "POST" || f.lastPath != "/api/devices/u1/boot" {
		t.Errorf("called %s %s", f.lastMethod, f.lastPath)
	}
	if _, err := f.client().bootDevice(raw(map[string]string{})); err == nil {
		t.Error("want error when udid missing")
	}
}

func TestLaunchApp(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	// Fresh by default: no reset=no in the query.
	if _, err := f.client().launchApp(raw(map[string]string{"udid": "u1", "app": "com.x"})); err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/devices/u1/app/launch" || f.lastQuery != "" || !strings.Contains(f.lastBody, "com.x") {
		t.Errorf("fresh launch: %s?%s body=%s", f.lastPath, f.lastQuery, f.lastBody)
	}
	// fresh=false ⇒ resume.
	no := false
	_, _ = f.client().launchApp(raw(deviceArgs{UDID: "u1", App: "com.x", Fresh: &no}))
	if f.lastQuery != "reset=no" {
		t.Errorf("resume query = %q", f.lastQuery)
	}
	if _, err := f.client().launchApp(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want error when app missing")
	}
}

func TestInstallApp(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	got, err := f.client().installApp(raw(map[string]string{"udid": "u1", "appFile": "/x/My.app"}))
	if err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/devices/u1/app/install" || !strings.Contains(f.lastBody, "/x/My.app") {
		t.Errorf("install: %s body=%s", f.lastPath, f.lastBody)
	}
	if !strings.Contains(got, "installed") {
		t.Errorf("result = %q", got)
	}
	f.failPost = true
	if _, err := f.client().installApp(raw(map[string]string{"udid": "u1", "appFile": "/x.app"})); err == nil {
		t.Error("want error when the install POST fails")
	}
	f.failPost = false
	if _, err := f.client().installApp(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want error when appFile missing")
	}
	if _, err := f.client().installApp(raw(map[string]string{"appFile": "/x.app"})); err == nil {
		t.Error("want error when udid missing")
	}
}

func TestHTTPErrorSurfaces(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.status, f.body = 502, "boom"
	if _, err := f.client().listDevices(nil); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("want 502 surfaced, got %v", err)
	}
	if _, err := f.client().bootDevice(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want post error surfaced")
	}
}

func TestTransportError(t *testing.T) {
	c := NewClient("http://127.0.0.1:0") // nothing listening
	if _, err := c.listDevices(nil); err == nil {
		t.Error("want transport error on GET")
	}
	if _, err := c.bootDevice(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want transport error on POST")
	}
}

func TestSchemas(t *testing.T) {
	if schemaNone()["type"] != "object" {
		t.Error("schemaNone")
	}
	if req, _ := schemaDevice()["required"].([]string); len(req) != 1 || req[0] != "udid" {
		t.Error("schemaDevice required")
	}
	// Both branches of schemaDeviceApp.
	for _, appRequired := range []bool{true, false} {
		s := schemaDeviceApp(appRequired)
		if _, ok := s["properties"].(map[string]any)["app"]; !ok {
			t.Errorf("schemaDeviceApp(%v) missing app", appRequired)
		}
	}
	l := schemaLaunch()
	if req, _ := l["required"].([]string); len(req) != 2 {
		t.Errorf("schemaLaunch required = %v", l["required"])
	}
	if deviceProp()["type"] != "string" || appProp()["type"] != "string" {
		t.Error("prop types")
	}
}

// errBodyResponse builds an http.Response whose body fails on Read, to cover
// readOK's read-error path.
type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (errReadCloser) Close() error             { return nil }

func TestReadOKBodyError(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: errReadCloser{}}
	if _, err := readOK(resp); err == nil {
		t.Error("want body read error surfaced")
	}
}

func TestUITreeAndLaunchServerError(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.status, f.body = 502, "down"
	if _, err := f.client().uiTree(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want uiTree error when server fails")
	}
	if _, err := f.client().launchApp(raw(map[string]string{"udid": "u1", "app": "com.x"})); err == nil {
		t.Error("want launchApp error when server fails")
	}
}

// a TestHive-shaped tree: root Application carries the screen size, then a
// field and a greeting.
const treeBody = `{"nodes":[
 {"type":"Application","frame":{"x":0,"y":0,"width":400,"height":800}},
 {"type":"TextField","identifier":"username-input","label":"Username","frame":{"x":40,"y":100,"width":320,"height":40}},
 {"type":"StaticText","label":"Hello, devicelab!","frame":{"x":40,"y":300,"width":320,"height":24}}
]}`

func TestTap(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = treeBody
	got, err := f.client().tap(raw(map[string]string{"udid": "u1", "testid": "username-input"}))
	if err != nil || got != "tapped username-input" {
		t.Fatalf("tap = %q, %v", got, err)
	}
	// Last call is the POST /tap with the resolved centre: (40+160)/400=0.5, (100+20)/800=0.15.
	if f.lastMethod != "POST" || f.lastPath != "/api/devices/u1/tap" {
		t.Errorf("tap called %s %s", f.lastMethod, f.lastPath)
	}
	if !strings.Contains(f.lastBody, `"x":0.5`) || !strings.Contains(f.lastBody, `"y":0.15`) {
		t.Errorf("tap point body = %s", f.lastBody)
	}
	// Missing args.
	if _, err := f.client().tap(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want error when testid missing")
	}
	// Element not on screen.
	if _, err := f.client().tap(raw(map[string]string{"udid": "u1", "testid": "nope"})); err == nil {
		t.Error("want error for unknown testid")
	}
	// Tree fetch fails.
	f.status = 502
	if _, err := f.client().tap(raw(map[string]string{"udid": "u1", "testid": "username-input"})); err == nil {
		t.Error("want error when tree fetch fails")
	}
}

func TestAssertVisible(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = treeBody
	c := f.client()
	if got, err := c.assertVisible(raw(map[string]string{"udid": "u1", "testid": "username-input"})); err != nil || !strings.Contains(got, "username-input") {
		t.Errorf("by testid = %q, %v", got, err)
	}
	if got, err := c.assertVisible(raw(map[string]string{"udid": "u1", "text": "hello, DEVICELAB"})); err != nil || !strings.Contains(got, "hello") {
		t.Errorf("by text (case-insensitive) = %q, %v", got, err)
	}
	if _, err := c.assertVisible(raw(map[string]string{"udid": "u1", "text": "not here"})); err == nil {
		t.Error("want error when absent")
	}
	if _, err := c.assertVisible(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want error when neither testid nor text given")
	}
	f.status = 502
	if _, err := c.assertVisible(raw(map[string]string{"udid": "u1", "testid": "x"})); err == nil {
		t.Error("want error when tree fetch fails")
	}
}

func TestTapPointEdges(t *testing.T) {
	// No screen dimensions.
	if _, _, err := tapPoint(nil, "x"); err == nil {
		t.Error("want error on empty tree")
	}
	zero := []treeNode{{Type: "Application"}}
	if _, _, err := tapPoint(zero, "x"); err == nil {
		t.Error("want error on zero-size screen")
	}
	// Off-screen frame clamps into 0-1.
	nodes := []treeNode{{Type: "Application"}}
	nodes[0].Frame.Width, nodes[0].Frame.Height = 100, 100
	off := treeNode{Identifier: "e"}
	off.Frame.X, off.Frame.Y, off.Frame.Width, off.Frame.Height = 500, -50, 10, 10
	nodes = append(nodes, off)
	x, y, err := tapPoint(nodes, "e")
	if err != nil || x != 1 || y != 0 {
		t.Errorf("clamp: x=%v y=%v err=%v", x, y, err)
	}
}

func TestFetchTreeBadJSON(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = "not json"
	if _, err := f.client().fetchTree("u1", ""); err == nil {
		t.Error("want error on unparseable tree")
	}
}

func TestSchemaTapAssert(t *testing.T) {
	if req, _ := schemaTap()["required"].([]string); len(req) != 2 {
		t.Errorf("schemaTap required = %v", schemaTap()["required"])
	}
	if req, _ := schemaAssert()["required"].([]string); len(req) != 1 {
		t.Errorf("schemaAssert required = %v", schemaAssert()["required"])
	}
}

func TestTapPostFails(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body, f.failPost = treeBody, true // tree resolves, the tap POST 502s
	if _, err := f.client().tap(raw(map[string]string{"udid": "u1", "testid": "username-input"})); err == nil {
		t.Error("want error when the tap POST fails")
	}
}

func TestFetchTreeWithApp(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = treeBody
	if _, err := f.client().fetchTree("u1", "com.x"); err != nil {
		t.Fatal(err)
	}
	if f.lastQuery != "app=com.x" {
		t.Errorf("fetchTree scoped query = %q", f.lastQuery)
	}
}

func TestScreenshot(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = "\x89PNGfakebytes" // stand-in PNG payload
	content, err := f.client().screenshot(raw(map[string]string{"udid": "u1"}))
	if err != nil || len(content) != 1 {
		t.Fatalf("screenshot = %v, %v", content, err)
	}
	item := content[0].(map[string]any)
	if item["type"] != "image" || item["mimeType"] != "image/png" {
		t.Errorf("content item = %v", item)
	}
	if item["data"] != "iVBOTmZha2VieXRlcw==" && item["data"].(string) == "" {
		t.Errorf("data not base64-encoded: %v", item["data"])
	}
	if f.lastMethod != "GET" || f.lastPath != "/api/devices/u1/screenshot" {
		t.Errorf("called %s %s", f.lastMethod, f.lastPath)
	}
	if _, err := f.client().screenshot(raw(map[string]string{})); err == nil {
		t.Error("want error when udid missing")
	}
	f.status = 502
	if _, err := f.client().screenshot(raw(map[string]string{"udid": "u1"})); err == nil {
		t.Error("want error when server fails")
	}
}
