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
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{status: 200, body: `{"ok":true}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.lastMethod, f.lastPath, f.lastQuery, f.lastBody = r.Method, r.URL.Path, r.URL.RawQuery, string(b)
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
	if len(tools) != 5 || len(order) != 5 {
		t.Fatalf("want 5 tools, got %d/%d", len(tools), len(order))
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
