package mcp

import (
	"strings"
	"testing"
)

// tree is a small tree-endpoint body: an application root plus the given
// element JSON fragments.
func tree(elems ...string) string {
	body := `{"nodes":[{"index":0,"type":"Application","frame":{"width":400,"height":800}}`
	if len(elems) > 0 {
		body += "," + strings.Join(elems, ",")
	}
	return body + `]}`
}

const loginNodes = `
{"index":1,"type":"TextField","identifier":"username-input","placeholder":"Username","frame":{"x":0,"y":100,"width":400,"height":40}},
{"index":2,"type":"SecureTextField","identifier":"password-input","placeholder":"Password","frame":{"x":0,"y":160,"width":400,"height":40}},
{"index":3,"type":"Button","identifier":"login-button","label":"Sign In","frame":{"x":0,"y":220,"width":400,"height":40}},
{"index":4,"type":"StaticText","label":"Welcome","frame":{"x":0,"y":40,"width":400,"height":30}}`

func TestSnapshotInteractive(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = tree(loginNodes)
	out, err := f.client().snapshot(raw(map[string]any{"udid": "U", "app": "com.x"}))
	if err != nil {
		t.Fatal(err)
	}
	// The three controls, each with a ref, role, name, testid, and centre;
	// the StaticText is excluded from interactive mode.
	if !strings.Contains(out, `e1 textbox "Username" testid=username-input @200,120`) {
		t.Errorf("username line wrong:\n%s", out)
	}
	if !strings.Contains(out, `e3 button "Sign In" testid=login-button @200,240`) {
		t.Errorf("button line wrong:\n%s", out)
	}
	if strings.Contains(out, "Welcome") {
		t.Errorf("interactive mode leaked text:\n%s", out)
	}
	// First snapshot marks every ref as new.
	if !strings.HasPrefix(out, "* e1") {
		t.Errorf("first snapshot should mark new refs:\n%s", out)
	}
}

func TestSnapshotFullIncludesText(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = tree(loginNodes)
	out, _ := f.client().snapshot(raw(map[string]any{"udid": "U", "mode": "full"}))
	if !strings.Contains(out, `text "Welcome"`) {
		t.Errorf("full mode missing text:\n%s", out)
	}
}

func TestSnapshotRefStability(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(loginNodes)
	first, _ := c.snapshot(raw(map[string]any{"udid": "U"}))
	// Same screen again: same refs, and no longer marked new.
	second, _ := c.snapshot(raw(map[string]any{"udid": "U"}))
	if !strings.Contains(second, `e3 button "Sign In"`) {
		t.Errorf("ref not reused:\n%s", second)
	}
	if strings.Contains(second, "* e") {
		t.Errorf("second snapshot should mark nothing new:\n%s", second)
	}
	_ = first
	// A different device keeps its own ref space.
	f.body = tree(`{"index":1,"type":"Button","identifier":"only","label":"Only","frame":{"x":0,"y":0,"width":10,"height":10}}`)
	other, _ := c.snapshot(raw(map[string]any{"udid": "OTHER"}))
	if !strings.Contains(other, "e1 button") {
		t.Errorf("second device should start its own refs at e1:\n%s", other)
	}
}

func TestSnapshotNewElementMarkedAndKeepsNumber(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(`{"index":1,"type":"Button","identifier":"a","label":"A","frame":{"x":0,"y":0,"width":10,"height":10}}`)
	_, _ = c.snapshot(raw(map[string]any{"udid": "U"}))
	// A second button appears; it must get e2 (e1 stays A's) and be marked new.
	f.body = tree(
		`{"index":1,"type":"Button","identifier":"a","label":"A","frame":{"x":0,"y":0,"width":10,"height":10}}`,
		`{"index":2,"type":"Button","identifier":"b","label":"B","frame":{"x":0,"y":20,"width":10,"height":10}}`)
	out, _ := c.snapshot(raw(map[string]any{"udid": "U"}))
	if !strings.Contains(out, "* e2 button \"B\"") {
		t.Errorf("new button should be e2 and marked new:\n%s", out)
	}
	if strings.Contains(out, "* e1") {
		t.Errorf("existing button e1 should not be re-marked new:\n%s", out)
	}
}

func TestSnapshotDiff(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree(`{"index":1,"type":"Button","identifier":"a","label":"A","frame":{"x":0,"y":0,"width":10,"height":10}}`)
	_, _ = c.snapshot(raw(map[string]any{"udid": "U"})) // establish baseline
	// Replace A with B.
	f.body = tree(`{"index":1,"type":"Button","identifier":"b","label":"B","frame":{"x":0,"y":0,"width":10,"height":10}}`)
	out, _ := c.snapshot(raw(map[string]any{"udid": "U", "mode": "diff"}))
	if !strings.Contains(out, "+ e2 button \"B\"") {
		t.Errorf("diff missing added B:\n%s", out)
	}
	if !strings.Contains(out, "- e1 button \"A\"") {
		t.Errorf("diff missing removed A:\n%s", out)
	}
	// A repeat shows the element as unchanged (=), with nothing added or removed.
	same, _ := c.snapshot(raw(map[string]any{"udid": "U", "mode": "diff"}))
	if !strings.Contains(same, "= e2 button \"B\"") ||
		strings.Contains(same, "+ ") || strings.Contains(same, "- ") {
		t.Errorf("repeat should be all =, got:\n%s", same)
	}
}

// The (no change) sentinel appears when a diff has no entries at all — an
// empty screen following an empty screen.
func TestSnapshotDiffNoChange(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	f.body = tree()
	_, _ = c.snapshot(raw(map[string]any{"udid": "U", "mode": "diff"}))
	out, _ := c.snapshot(raw(map[string]any{"udid": "U", "mode": "diff"}))
	if out != "(no change)" {
		t.Errorf("expected no change, got:\n%s", out)
	}
}

func TestSnapshotSurfacesDialog(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = tree(
		`{"index":1,"type":"Alert","label":"Allow notifications?","frame":{"x":40,"y":300,"width":320,"height":160}}`,
		`{"index":2,"type":"Button","identifier":"allow","label":"Allow","frame":{"x":40,"y":420,"width":320,"height":40}}`)
	out, _ := f.client().snapshot(raw(map[string]any{"udid": "U"}))
	if !strings.HasPrefix(out, `dialog: alertdialog "Allow notifications?"`) {
		t.Errorf("dialog not surfaced first:\n%s", out)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = tree()
	out, _ := f.client().snapshot(raw(map[string]any{"udid": "U"}))
	if out != "(no elements)" {
		t.Errorf("empty screen: %q", out)
	}
}

func TestSnapshotErrors(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	c := f.client()
	if _, err := c.snapshot(raw(map[string]any{})); err == nil {
		t.Error("missing udid must error")
	}
	if _, err := c.snapshot([]byte("{bad")); err == nil {
		t.Error("bad json must error")
	}
	f.status = 500
	f.body = "boom"
	if _, err := c.snapshot(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("server error must propagate")
	}
	// A 200 with a non-JSON body is a decode error.
	f.status = 200
	f.body = "not json"
	if _, err := c.snapshot(raw(map[string]any{"udid": "U"})); err == nil {
		t.Error("undecodable tree must error")
	}
}
