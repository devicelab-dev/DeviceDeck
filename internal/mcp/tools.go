package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin HTTP client for a running `devicedeck serve`. Every tool
// is a wrapper over one of its endpoints, so the MCP adds no device logic of
// its own.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient targets a running server (default http://127.0.0.1:8787).
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Tools returns the DeviceDeck tool set and a stable listing order: device
// lifecycle (list, boot, launch), inspection (ui_tree, screenshot),
// selector-based acting (tap, assert_visible), and the device page URL to
// drive typing-heavy flows with the agent's own web tools. Each is a thin
// wrapper over the server's HTTP API.
func (c *Client) Tools() (map[string]Tool, []string) {
	tools := map[string]Tool{
		"list_devices":    {Description: descListDevices, InputSchema: schemaNone(), Call: c.listDevices},
		"device_page_url": {Description: descPageURL, InputSchema: schemaDeviceApp(true), Call: c.devicePageURL},
		"ui_tree":         {Description: descUITree, InputSchema: schemaDeviceApp(false), Call: c.uiTree},
		"boot_device":     {Description: descBoot, InputSchema: schemaDevice(), Call: c.bootDevice},
		"install_app":     {Description: descInstall, InputSchema: schemaInstall(), Call: c.installApp},
		"launch_app":      {Description: descLaunch, InputSchema: schemaLaunch(), Call: c.launchApp},
		"tap":             {Description: descTap, InputSchema: schemaTap(), Call: c.tap},
		"assert_visible":  {Description: descAssert, InputSchema: schemaAssert(), Call: c.assertVisible},
		"screenshot":      {Description: descScreenshot, InputSchema: schemaDevice(), Raw: c.screenshot},
	}
	order := []string{"list_devices", "device_page_url", "ui_tree", "boot_device", "install_app", "launch_app", "tap", "assert_visible", "screenshot"}
	return tools, order
}

// deviceArgs is the shape the tools take: which device, which app to scope a
// tree to, the fresh/resume toggle, and the selector fields the act tools use.
type deviceArgs struct {
	UDID    string `json:"udid"`
	App     string `json:"app"`
	AppFile string `json:"appFile"`
	Fresh   *bool  `json:"fresh"`
	Testid  string `json:"testid"`
	Text    string `json:"text"`
}

// treeNode is the subset of a mirrored element the act tools read: its
// identifier (data-testid), accessible text, and on-device frame. Decoded
// straight from the tree endpoint's JSON, so the MCP stays a thin adapter
// over the wire shape rather than over the runner's Go types.
type treeNode struct {
	Identifier string `json:"identifier"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Type       string `json:"type"`
	Frame      struct {
		X, Y, Width, Height float64
	} `json:"frame"`
}

func decodeArgs(raw json.RawMessage) (deviceArgs, error) {
	var a deviceArgs
	if len(raw) == 0 {
		return a, nil
	}
	err := json.Unmarshal(raw, &a)
	return a, err
}

func (c *Client) listDevices(json.RawMessage) (string, error) {
	body, err := c.get("/api/devices")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// devicePageURL is pure — no call. It returns the mirror URL an agent opens
// with its own browser tools to drive the device by selector.
func (c *Client) devicePageURL(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	u := fmt.Sprintf("%s/device/%s", c.BaseURL, url.PathEscape(a.UDID))
	if a.App != "" {
		u += "?app=" + url.QueryEscape(a.App)
	}
	return u, nil
}

func (c *Client) uiTree(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	path := "/api/devices/" + url.PathEscape(a.UDID) + "/tree"
	if a.App != "" {
		path += "?app=" + url.QueryEscape(a.App)
	}
	body, err := c.get(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) bootDevice(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/boot", nil); err != nil {
		return "", err
	}
	return "booted " + a.UDID, nil
}

// launchApp launches an app fresh by default — a clean first-run screen, the
// slate a new agent session expects — matching the server's own default.
// Pass fresh=false to resume the app where it was left.
func (c *Client) launchApp(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || a.App == "" {
		return "", fmt.Errorf("udid and app are required")
	}
	path := "/api/devices/" + url.PathEscape(a.UDID) + "/app/launch"
	if a.Fresh != nil && !*a.Fresh {
		path += "?reset=no"
	}
	body, _ := json.Marshal(map[string]string{"app": a.App})
	if _, err := c.post(path, body); err != nil {
		return "", err
	}
	return fmt.Sprintf("launched %s on %s", a.App, a.UDID), nil
}

func (c *Client) installApp(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || a.AppFile == "" {
		return "", fmt.Errorf("udid and appFile are required")
	}
	path := "/api/devices/" + url.PathEscape(a.UDID) + "/app/install"
	body, _ := json.Marshal(map[string]string{"appFile": a.AppFile})
	if _, err := c.post(path, body); err != nil {
		return "", err
	}
	return fmt.Sprintf("installed %s on %s", a.AppFile, a.UDID), nil
}

// tap resolves a data-testid to a point on the device and taps it. The
// resolution is deterministic — the element's own frame, by identifier — so
// what the agent taps is reviewable, the same durable selector a captured
// flow uses, not a coordinate it guessed.
func (c *Client) tap(raw json.RawMessage) (string, error) {
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
	body, _ := json.Marshal(map[string]float64{"x": x, "y": y})
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/tap", body); err != nil {
		return "", err
	}
	return "tapped " + a.Testid, nil
}

// assertVisible reports whether an element is on screen, by testid (exact
// identifier) or text (a substring of a label or value). A read-only check
// against the device's own tree — the assertion an agent records is one that
// holds on real hardware too.
func (c *Client) assertVisible(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" || (a.Testid == "" && a.Text == "") {
		return "", fmt.Errorf("udid and one of testid or text are required")
	}
	nodes, err := c.fetchTree(a.UDID, a.App)
	if err != nil {
		return "", err
	}
	target := a.Testid
	if target == "" {
		target = a.Text
	}
	if nodeVisible(nodes, a.Testid, a.Text) {
		return "visible: " + target, nil
	}
	return "", fmt.Errorf("not visible: %q", target)
}

// fetchTree pulls and decodes the device's UI tree.
func (c *Client) fetchTree(udid, app string) ([]treeNode, error) {
	path := "/api/devices/" + url.PathEscape(udid) + "/tree"
	if app != "" {
		path += "?app=" + url.QueryEscape(app)
	}
	body, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var t struct {
		Nodes []treeNode `json:"nodes"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, err
	}
	return t.Nodes, nil
}

// tapPoint resolves a data-testid to a normalized (0-1) tap point: the
// element's centre over the device's screen dimensions, which the tree's root
// (Application) node carries. Errors if the screen has no size or the element
// is absent.
func tapPoint(nodes []treeNode, testid string) (float64, float64, error) {
	if len(nodes) == 0 || nodes[0].Frame.Width <= 0 || nodes[0].Frame.Height <= 0 {
		return 0, 0, fmt.Errorf("no screen dimensions in tree")
	}
	w, h := nodes[0].Frame.Width, nodes[0].Frame.Height
	for _, n := range nodes {
		if n.Identifier == testid {
			return clamp01((n.Frame.X + n.Frame.Width/2) / w), clamp01((n.Frame.Y + n.Frame.Height/2) / h), nil
		}
	}
	return 0, 0, fmt.Errorf("no element with testid %q on screen", testid)
}

// nodeVisible reports whether the tree holds an element matching a testid
// (exact identifier) or text (case-insensitive substring of a label or value).
func nodeVisible(nodes []treeNode, testid, text string) bool {
	for _, n := range nodes {
		if testid != "" && n.Identifier == testid {
			return true
		}
		if text != "" && (containsFold(n.Label, text) || containsFold(n.Value, text)) {
			return true
		}
	}
	return false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// screenshot returns the device's current screen as an MCP image content
// item, so an agent that reasons over pixels can see the device — the
// complement to ui_tree for one that reasons over structure. Both platforms
// return PNG.
func (c *Client) screenshot(raw json.RawMessage) ([]any, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return nil, fmt.Errorf("udid is required")
	}
	body, err := c.get("/api/devices/" + url.PathEscape(a.UDID) + "/screenshot")
	if err != nil {
		return nil, err
	}
	return []any{map[string]any{
		"type":     "image",
		"data":     base64.StdEncoding.EncodeToString(body),
		"mimeType": "image/png",
	}}, nil
}

// get issues a GET and returns the body, turning a non-2xx into an error
// carrying the server's own message so the agent sees why it failed.
func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.HTTP.Get(c.BaseURL + path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return readOK(resp)
}

func (c *Client) post(path string, body []byte) ([]byte, error) {
	resp, err := c.HTTP.Post(c.BaseURL+path, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return readOK(resp)
}

func readOK(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
