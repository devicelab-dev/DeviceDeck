package mcp

import (
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

// Tools returns the DeviceDeck tool set and a stable listing order. Read and
// device-lifecycle tools only for now: they are what a browser-driving agent
// cannot do for itself (list, boot, launch, inspect), and handing back the
// device page URL lets it drive the rest with its own web tools.
func (c *Client) Tools() (map[string]Tool, []string) {
	tools := map[string]Tool{
		"list_devices":    {Description: descListDevices, InputSchema: schemaNone(), Call: c.listDevices},
		"device_page_url": {Description: descPageURL, InputSchema: schemaDeviceApp(true), Call: c.devicePageURL},
		"ui_tree":         {Description: descUITree, InputSchema: schemaDeviceApp(false), Call: c.uiTree},
		"boot_device":     {Description: descBoot, InputSchema: schemaDevice(), Call: c.bootDevice},
		"launch_app":      {Description: descLaunch, InputSchema: schemaLaunch(), Call: c.launchApp},
	}
	order := []string{"list_devices", "device_page_url", "ui_tree", "boot_device", "launch_app"}
	return tools, order
}

// deviceArgs is the shape most tools take: which device, and (where a tree is
// involved) which app to scope it to.
type deviceArgs struct {
	UDID  string `json:"udid"`
	App   string `json:"app"`
	Fresh *bool  `json:"fresh"`
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

// get issues a GET and returns the body, turning a non-2xx into an error
// carrying the server's own message so the agent sees why it failed.
func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.HTTP.Get(c.BaseURL + path)
	if err != nil {
		return nil, err
	}
	return readOK(resp)
}

func (c *Client) post(path string, body []byte) ([]byte, error) {
	resp, err := c.HTTP.Post(c.BaseURL+path, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	return readOK(resp)
}

func readOK(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
