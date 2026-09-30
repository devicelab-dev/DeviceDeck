package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// settingsArgs is what device_settings takes. Everything but udid goes to
// the server as is; the server validates it, so the rules live in one place.
type settingsArgs struct {
	UDID       string          `json:"udid"`
	Appearance string          `json:"appearance,omitempty"`
	Location   json.RawMessage `json:"location,omitempty"`
	App        string          `json:"app,omitempty"`
	Grant      []string        `json:"grant,omitempty"`
	Revoke     []string        `json:"revoke,omitempty"`
}

// deviceSettings sets dark mode, a simulated location, and app permissions
// through the server's settings route, and reports what it applied.
func (c *Client) deviceSettings(raw json.RawMessage) (string, error) {
	var a settingsArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", err
		}
	}
	if a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	body, _ := json.Marshal(a) // plain fields; cannot fail
	resp, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/settings", body)
	if err != nil {
		return "", err
	}
	var out struct {
		Applied []string `json:"applied"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return "", fmt.Errorf("decode settings result: %w", err)
	}
	return "set " + strings.Join(out.Applied, "; "), nil
}
