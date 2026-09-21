package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/uisem"
)

// longPress resolves a testid to its centre and holds a press there, for the
// menus and reorder handles a quick tap does not trigger. Same deterministic
// resolution as tap — the element's own frame, not a guessed coordinate.
func (c *Client) longPress(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || a.Testid == "" {
		return "", fmt.Errorf("udid and testid are required")
	}
	nodes, err := c.fetchTree(a.UDID, a.App)
	if err != nil {
		return "", err
	}
	x, y, err := tapPoint(nodes, a.Testid)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"x": x, "y": y, "durationMs": longPressMs})
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/tap", body); err != nil {
		return "", err
	}
	return "long-pressed " + a.Testid, nil
}

const longPressMs = 700

// swipeVectors maps a direction to a start and end point in normalized
// screen space: a gesture across the middle of the screen, long enough to
// scroll a list or page a carousel, short of the edges so it does not pull
// down a system surface.
var swipeVectors = map[string][4]float64{
	"up":    {0.5, 0.7, 0.5, 0.3},
	"down":  {0.5, 0.3, 0.5, 0.7},
	"left":  {0.7, 0.5, 0.3, 0.5},
	"right": {0.3, 0.5, 0.7, 0.5},
}

// swipe scrolls the screen in a direction. Content moves opposite the finger,
// so "up" reveals what is below — the direction names where the content goes,
// which is how a person describes a scroll.
func (c *Client) swipe(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	v, ok := swipeVectors[a.Direction]
	if !ok {
		return "", fmt.Errorf("direction must be one of up, down, left, right")
	}
	body, _ := json.Marshal(map[string]float64{"x1": v[0], "y1": v[1], "x2": v[2], "y2": v[3]})
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/swipe", body); err != nil {
		return "", err
	}
	return "swiped " + a.Direction, nil
}

// pressGestures maps a press name to how the server delivers it: the system
// gestures go through /gesture, Enter through /key as its HID usage.
var pressGestures = map[string]string{
	"home": "home", "appSwitcher": "appSwitcher",
	"notifications": "notificationCenter", "lock": "lockScreen",
}

const enterUsage = 0x28 // USB HID keyboard usage for Return

// press delivers a hardware-style key or system gesture the mirror has no
// on-screen button for: home, appSwitcher, notifications, lock, or enter.
func (c *Client) press(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || a.Key == "" {
		return "", fmt.Errorf("udid and key are required")
	}
	base := "/api/devices/" + url.PathEscape(a.UDID)
	if a.Key == "enter" {
		body, _ := json.Marshal(map[string]uint32{"usage": enterUsage})
		if _, err := c.post(base+"/key", body); err != nil {
			return "", err
		}
		return "pressed enter", nil
	}
	kind, ok := pressGestures[a.Key]
	if !ok {
		return "", fmt.Errorf("key must be one of home, appSwitcher, notifications, lock, enter")
	}
	body, _ := json.Marshal(map[string]string{"kind": kind})
	if _, err := c.post(base+"/gesture", body); err != nil {
		return "", err
	}
	return "pressed " + a.Key, nil
}

// findElement searches the current screen for elements matching any of the
// given criteria — testid, exact role, a substring of the name, or a
// substring of visible text — and returns each as a line an agent can act on
// (its testid and centre). It answers "is the thing I want here, and how do I
// address it" without dumping the whole tree.
func (c *Client) findElement(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	if a.Testid == "" && a.Role == "" && a.Name == "" && a.Text == "" {
		return "", fmt.Errorf("give at least one of testid, role, name, or text to match")
	}
	nodes, err := c.fetchTree(a.UDID, a.App)
	if err != nil {
		return "", err
	}
	var w, h float64
	if len(nodes) > 0 {
		w, h = nodes[0].Frame.Width, nodes[0].Frame.Height
	}
	var out []string
	for _, n := range nodes {
		if !matchesQuery(n, a) {
			continue
		}
		out = append(out, findLine(n, w, h))
	}
	if len(out) == 0 {
		return "", fmt.Errorf("no element matched")
	}
	return strings.Join(out, "\n"), nil
}

// matchesQuery reports whether a node satisfies every criterion the caller
// set (unset criteria are ignored), so combining role and name narrows.
func matchesQuery(n treeNode, a deviceArgs) bool {
	if a.Testid != "" && n.Identifier != a.Testid {
		return false
	}
	if a.Role != "" && uisem.Role(n.Type) != a.Role {
		return false
	}
	if a.Name != "" && !containsFold(n.Label, a.Name) && !containsFold(n.Value, a.Name) {
		return false
	}
	if a.Text != "" && !containsFold(n.Label, a.Text) && !containsFold(n.Value, a.Text) {
		return false
	}
	return true
}

// findLine renders a matched element: role, name, testid, and on-device
// centre when the screen size is known.
func findLine(n treeNode, w, h float64) string {
	name := n.Label
	if name == "" {
		name = n.Value
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %q", roleOrType(n.Type), name)
	if n.Identifier != "" {
		fmt.Fprintf(&b, " testid=%s", n.Identifier)
	}
	if w > 0 && h > 0 {
		fmt.Fprintf(&b, " @%.2f,%.2f", clamp01((n.Frame.X+n.Frame.Width/2)/w), clamp01((n.Frame.Y+n.Frame.Height/2)/h))
	}
	return b.String()
}

// roleOrType names an element by its role, falling back to its raw type when
// it carries none, so a match is never blank.
func roleOrType(t string) string {
	if r := uisem.Role(t); r != "" {
		return r
	}
	return t
}

// openURL opens a link on the device — an https URL or a custom deep-link
// scheme — so an agent can jump a flow straight to a screen the app exposes
// by URL instead of navigating there tap by tap.
func (c *Client) openURL(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || a.URL == "" {
		return "", fmt.Errorf("udid and url are required")
	}
	body, _ := json.Marshal(map[string]string{"url": a.URL})
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/openurl", body); err != nil {
		return "", err
	}
	return "opened " + a.URL, nil
}
