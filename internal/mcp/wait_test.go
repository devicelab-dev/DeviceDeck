package mcp

import (
	"strings"
	"testing"
	"time"
)

func fastWait(t *testing.T) {
	t.Helper()
	prev := waitPoll
	waitPoll = time.Millisecond
	t.Cleanup(func() { waitPoll = prev })
}

// visible and gone succeed when the screen matches and fail — never
// succeed — when the timeout runs out.
func TestWaitForVisibleAndGone(t *testing.T) {
	fastWait(t)
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	tests := []struct {
		name    string
		args    map[string]any
		wantOut string
		wantErr string
	}{
		{"visible by testid", map[string]any{"testid": "username"}, "username is visible", ""},
		{"visible by text, several match", map[string]any{"text": "add"}, "add is visible", ""},
		{"visible by role and name", map[string]any{"role": "button", "name": "sign"}, "button sign is visible", ""},
		{"gone already", map[string]any{"testid": "spinner", "state": "gone"}, "spinner is gone", ""},
		{"never appears", map[string]any{"testid": "spinner", "timeoutMs": 20}, "", "timed out"},
		{"never goes", map[string]any{"testid": "username", "state": "gone", "timeoutMs": 20}, "", "timed out"},
		{"no target", map[string]any{}, "", "name the element"},
		{"bad state", map[string]any{"testid": "x", "state": "shiny"}, "", "state must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args["udid"] = "U"
			out, err := c.waitFor(raw(tt.args))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || out != tt.wantOut {
				t.Errorf("out = %q, err = %v, want %q", out, err, tt.wantOut)
			}
		})
	}
}

// A ref waits on the element it named; an unknown ref is an error, not a wait.
func TestWaitForRef(t *testing.T) {
	fastWait(t)
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	snap, _ := c.snapshot(raw(map[string]any{"udid": "U"}))
	ref := refFor(t, snap, `"Sign In"`)
	if out, err := c.waitFor(raw(map[string]any{"udid": "U", "ref": ref})); err != nil || !strings.Contains(out, "visible") {
		t.Errorf("ref visible: %q %v", out, err)
	}
	api.tree = tree()
	if out, err := c.waitFor(raw(map[string]any{"udid": "U", "ref": ref, "state": "gone"})); err != nil || !strings.Contains(out, "gone") {
		t.Errorf("ref gone: %q %v", out, err)
	}
	if _, err := c.waitFor(raw(map[string]any{"udid": "U", "ref": "e999"})); err == nil || !strings.Contains(err.Error(), "unknown ref") {
		t.Errorf("unknown ref: %v", err)
	}
}

// settled uses the server's settle barrier; a screen that cannot be read
// says so, distinctly from an element being absent.
func TestWaitForSettledAndUnreadable(t *testing.T) {
	fastWait(t)
	api := newRoutedAPI(formTree())
	defer api.srv.Close()
	c := api.client()
	if out, err := c.waitFor(raw(map[string]any{"udid": "U", "app": "com.x", "state": "settled"})); err != nil || out != "screen settled" {
		t.Errorf("settled: %q %v", out, err)
	}
	if got := api.last("/tree"); !strings.Contains(got, "GET /api/devices/U/tree") {
		t.Errorf("settle call = %q", got)
	}
	api.fail["/tree"] = true
	for _, state := range []string{"settled", "visible"} {
		_, err := c.waitFor(raw(map[string]any{"udid": "U", "testid": "x", "state": state}))
		if err == nil || !strings.Contains(err.Error(), "could not read the device's screen") {
			t.Errorf("%s on an unreadable screen: %v", state, err)
		}
	}
	if _, err := c.waitFor(raw(map[string]any{"state": "settled"})); err == nil {
		t.Error("missing udid must error")
	}
	if _, err := c.waitFor([]byte("{bad")); err == nil {
		t.Error("bad arguments must error")
	}
}

func TestWaitTimeoutClamps(t *testing.T) {
	if waitTimeout(0) != waitDefault || waitTimeout(-5) != waitDefault {
		t.Error("default")
	}
	if waitTimeout(500) != 500*time.Millisecond || waitTimeout(10*60*1000) != waitMax {
		t.Error("clamp")
	}
}
