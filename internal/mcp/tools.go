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
	// Token is the server's access token (devicedeck --token), sent on
	// every call and carried by the device-page links it hands out.
	Token        string
	HTTP         *http.Client
	snaps        *snapshotState
	AppSkillsDir string
	// OutDir is where results too large to return inline are written.
	OutDir string
}

// callTimeout bounds one request to the server. A cold emulator boot waits up
// to three minutes for Android to finish booting before the driver can start,
// so the bound sits above that rather than failing a boot that is still going.
const callTimeout = 4 * time.Minute

// NewClient targets a running server (default http://127.0.0.1:8787).
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		HTTP:         &http.Client{Timeout: callTimeout},
		snaps:        newSnapshotState(),
		AppSkillsDir: appSkillsDir(),
		OutDir:       outDir(),
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
		"list_apps":       {Description: descListApps, InputSchema: schemaOptionalDevice(), Call: c.listApps},
		"device_page_url": {Description: descPageURL, InputSchema: schemaDeviceApp(true), Call: c.devicePageURL},
		"ui_tree":         {Description: descUITree, InputSchema: schemaDeviceApp(false), Call: c.uiTree},
		"snapshot":        {Description: descSnapshot, InputSchema: schemaSnapshot(), Call: c.snapshot},
		"boot_device":     {Description: descBoot, InputSchema: schemaDevice(), Call: c.bootDevice},
		"install_app":     {Description: descInstall, InputSchema: schemaInstall(), Call: c.installApp},
		"launch_app":      {Description: descLaunch, InputSchema: schemaLaunch(), Call: c.launchApp},
		"tap":             {Description: descTap, InputSchema: schemaTarget(), Call: c.tap},
		"fill":            {Description: descFill, InputSchema: schemaFill(), Call: c.fill},
		"long_press":      {Description: descLongPress, InputSchema: schemaTarget(), Call: c.longPress},
		"swipe":           {Description: descSwipe, InputSchema: schemaSwipe(), Call: c.swipe},
		"press":           {Description: descPress, InputSchema: schemaPress(), Call: c.press},
		"find_element":    {Description: descFindElement, InputSchema: schemaFind(), Call: c.findElement},
		"wait_for":        {Description: descWaitFor, InputSchema: schemaWaitFor(), Call: c.waitFor},
		"app_skills":      {Description: descAppSkills, InputSchema: schemaApp(), Call: c.appSkillsTool},
		"open_url":        {Description: descOpenURL, InputSchema: schemaOpenURL(), Call: c.openURL},
		"device_settings": {Description: descSettings, InputSchema: schemaSettings(), Call: c.deviceSettings},
		"assert_visible":  {Description: descAssert, InputSchema: schemaAssert(), Call: c.assertVisible},
		"screenshot":      {Description: descScreenshot, InputSchema: schemaScreenshot(), Raw: c.screenshot},
	}
	order := []string{"list_devices", "list_apps", "device_page_url", "ui_tree", "snapshot", "boot_device", "install_app", "launch_app", "tap", "fill", "long_press", "swipe", "press", "find_element", "wait_for", "app_skills", "open_url", "device_settings", "assert_visible", "screenshot"}
	return tools, order
}

// deviceArgs is the shape the tools take: which device, which app to scope a
// tree to, the fresh/resume toggle, and the selector fields the act tools use.
type deviceArgs struct {
	UDID      string `json:"udid"`
	App       string `json:"app"`
	AppFile   string `json:"appFile"`
	Fresh     *bool  `json:"fresh"`
	Testid    string `json:"testid"`
	Text      string `json:"text"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Direction string `json:"direction"`
	Key       string `json:"key"`
	URL       string `json:"url"`
	// Ref is an element ref from snapshot (e12).
	Ref string `json:"ref"`
	// Value is what fill types; Submit presses Enter after it.
	Value  string `json:"value"`
	Submit bool   `json:"submit"`
	// Full asks screenshot for the original PNG instead of the capped JPEG.
	Full bool `json:"full"`
}

// treeNode is the subset of a mirrored element the act tools read: its
// identifier (data-testid), accessible text, and on-device frame. Decoded
// straight from the tree endpoint's JSON, so the MCP stays a thin adapter
// over the wire shape rather than over the runner's Go types.
type treeNode struct {
	// chrome is set on the root node only; see fetchTree.
	chrome      chrome
	Index       int    `json:"index"`
	ParentIndex *int   `json:"parentIndex"`
	Identifier  string `json:"identifier"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	Placeholder string `json:"placeholder"`
	Type        string `json:"type"`
	Enabled     *bool  `json:"enabled"`
	Frame       struct {
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

// listApps returns the registered builds: all of them, or those that suit
// one device, marked installed or launched there.
func (c *Client) listApps(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil {
		return "", err
	}
	path := "/api/apps"
	if a.UDID != "" {
		path = "/api/devices/" + url.PathEscape(a.UDID) + "/apps"
	}
	body, err := c.get(path)
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
	q := url.Values{}
	if a.App != "" {
		q.Set("app", a.App)
	}
	if c.Token != "" {
		q.Set("token", c.Token) // a browser opening the page gets in with it
	}
	u := fmt.Sprintf("%s/device/%s", c.BaseURL, url.PathEscape(a.UDID))
	if len(q) > 0 {
		u += "?" + q.Encode()
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
	return c.spillJSON("ui_tree", string(body))
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
	msg := fmt.Sprintf("launched %s on %s", a.App, a.UDID)
	if skills := appSkills(c.AppSkillsDir, a.App); skills != "" {
		msg += "\n" + skills
	}
	return msg, nil
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

// tap finds an element — by snapshot ref, testid, visible text, or role and
// name — and taps its centre. The element is resolved against the device's
// current tree, so what the agent taps is a reviewable selector, not a
// coordinate it guessed.
func (c *Client) tap(raw json.RawMessage) (string, error) {
	a, err := decodeArgs(raw)
	if err != nil || a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	x, y, err := c.actionPoint(a)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]float64{"x": x, "y": y})
	if _, err := c.post("/api/devices/"+url.PathEscape(a.UDID)+"/tap", body); err != nil {
		return "", err
	}
	return "tapped " + targetName(a), nil
}

// actionPoint resolves the element a tool names to its centre, refusing a
// disabled one: acting on it would do nothing.
func (c *Client) actionPoint(a deviceArgs) (float64, float64, error) {
	n, nodes, err := c.resolveTarget(a)
	if err != nil {
		return 0, 0, err
	}
	if isDisabled(n) {
		return 0, 0, errDisabled(a)
	}
	if err := reachable(nodes, n, a); err != nil {
		return 0, 0, err
	}
	return centre(nodes, n)
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
		return nil, unreadable(err)
	}
	var t struct {
		Nodes  []treeNode `json:"nodes"`
		Chrome chrome     `json:"chrome"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, unreadable(err)
	}
	if len(t.Nodes) > 0 {
		t.Nodes[0].chrome = t.Chrome
	}
	return t.Nodes, nil
}

// chrome is the system UI the server reports over the app (Android's status
// bar and keyboard, which its tree leaves out), carried on the root node so
// every caller of fetchTree has it without a second return value.
type chrome struct {
	StatusBar   float64 `json:"statusBar"`
	KeyboardTop float64 `json:"keyboardTop"`
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
	data, mime := body, "image/png"
	if !a.Full {
		if data, mime, err = agentImage(body, agentImageMax); err != nil {
			return nil, err
		}
	}
	return []any{map[string]any{
		"type":     "image",
		"data":     base64.StdEncoding.EncodeToString(data),
		"mimeType": mime,
	}}, nil
}

// get issues a GET and returns the body, turning a non-2xx into an error
// carrying the server's own message so the agent sees why it failed.
func (c *Client) get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *Client) post(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPost, path, body)
}

// do sends one request to the server, with the access token when one is set.
func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
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
