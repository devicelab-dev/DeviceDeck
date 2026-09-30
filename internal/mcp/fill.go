package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
)

// fill sets a text field to a value through the device's driver — the same
// path the device page's fill() takes, which types, reads the field back and
// fails if the device does not hold the value. It lets an agent with no
// browser tool type into an app. Submit presses Enter afterwards.
func (c *Client) fill(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	n, nodes, err := c.resolveTarget(a)
	if err != nil {
		return "", err
	}
	if isDisabled(n) {
		return "", errDisabled(a)
	}
	if err := reachable(nodes, n, a); err != nil {
		return "", err
	}
	body, err := fillAction(nodes, n, a)
	if err != nil {
		return "", err
	}
	if err := c.act(a.UDID, body); err != nil {
		return "", fmt.Errorf("fill %s: %w", targetName(a), err)
	}
	if a.Submit {
		if err := c.pressEnter(a.UDID); err != nil {
			return "", err
		}
		return fmt.Sprintf("filled %s and pressed enter", targetName(a)), nil
	}
	return "filled " + targetName(a), nil
}

// fillAction builds the /act request for a fill: the field's identifier when
// it is unique on screen (the driver finds it by id), its centre both as a
// screen fraction (for Flow Capture) and in the tree's own units (for the
// driver), the value, and how much the field held (to size the clear).
func fillAction(nodes []treeNode, n treeNode, a deviceArgs) ([]byte, error) {
	x, y, err := centre(nodes, n)
	if err != nil {
		return nil, err
	}
	req := map[string]any{
		"id": newActionID(), "kind": "fill", "app": a.App,
		"x": x, "y": y,
		"px": n.Frame.X + n.Frame.Width/2, "py": n.Frame.Y + n.Frame.Height/2,
		"text": a.Value, "prev": len([]rune(n.Value)),
	}
	if n.Identifier != "" && countID(nodes, n.Identifier) == 1 {
		req["field"] = n.Identifier
	}
	return json.Marshal(req)
}

// countID counts the nodes carrying an identifier.
func countID(nodes []treeNode, id string) int {
	count := 0
	for _, n := range nodes {
		if n.Identifier == id {
			count++
		}
	}
	return count
}

// act posts one action to the server's /act and reports the driver's own
// error, which comes back inside a successful reply.
func (c *Client) act(udid string, body []byte) error {
	resp, err := c.post("/api/devices/"+url.PathEscape(udid)+"/act", body)
	if err != nil {
		return err
	}
	var out struct {
		Act struct {
			Error    string `json:"error"`
			TimedOut bool   `json:"timedOut"`
		} `json:"act"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return fmt.Errorf("decode action result: %w", err)
	}
	if out.Act.Error != "" {
		return fmt.Errorf("%s", out.Act.Error)
	}
	return nil
}

// pressEnter delivers Return, as the press tool's "enter" does.
func (c *Client) pressEnter(udid string) error {
	body, _ := json.Marshal(map[string]uint32{"usage": enterUsage})
	_, err := c.post("/api/devices/"+url.PathEscape(udid)+"/key", body)
	return err
}

// newActionID is a fresh id for /act, whose repeat of an id returns the
// earlier result instead of acting twice — so a retried request cannot type
// the value twice.
func newActionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand does not fail on supported platforms
	return "mcp-" + hex.EncodeToString(b)
}
