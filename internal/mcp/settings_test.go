package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeviceSettings(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	f.body = `{"ok":true,"applied":["appearance dark","grant camera"]}`
	got, err := f.client().deviceSettings(raw(map[string]any{
		"udid": "U 1", "appearance": "dark", "app": "com.x", "grant": []string{"camera"},
	}))
	if err != nil || got != "set appearance dark; grant camera" {
		t.Fatalf("got %q, %v", got, err)
	}
	if f.lastPath != "/api/devices/U 1/settings" {
		t.Errorf("path = %q", f.lastPath)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(f.lastBody), &sent)
	if sent["appearance"] != "dark" || sent["app"] != "com.x" || sent["location"] != nil || sent["revoke"] != nil {
		t.Errorf("body = %s", f.lastBody)
	}
}

func TestDeviceSettingsErrors(t *testing.T) {
	f := newFakeAPI()
	defer f.close()
	tests := []struct {
		name, want string
		args       json.RawMessage
		status     int
		body       string
	}{
		{"no udid", "udid is required", nil, 200, ""},
		{"bad args", "cannot unmarshal", json.RawMessage(`["x"]`), 200, ""},
		{"server refuses", "400", raw(map[string]any{"udid": "U"}), 400, `{"ok":false,"error":"nothing to set"}`},
		{"bad reply", "decode settings result", raw(map[string]any{"udid": "U"}), 200, `not json`},
	}
	for _, tt := range tests {
		f.status, f.body = tt.status, tt.body
		if _, err := f.client().deviceSettings(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err %v, want %q", tt.name, err, tt.want)
		}
	}
}
