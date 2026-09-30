package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// wait_for waits for a condition on the device's screen instead of a fixed
// sleep: an element to appear, an element to go (React Native removes hidden
// UI from the tree rather than hiding it, so "gone" is how a spinner or a
// dialog is known to be over), or the screen to stop changing. A wait that
// runs out fails — reporting success would let the next step act on a screen
// that is not the one the agent expects.

// Bounds on a wait.
const (
	waitDefault = 10 * time.Second
	waitMax     = 60 * time.Second
)

// waitPoll is how often a visible/gone wait re-reads the screen.
var waitPoll = 250 * time.Millisecond

// waitArgs is what wait_for takes: the element (named like tap), the state
// to wait for, and a timeout.
type waitArgs struct {
	deviceArgs
	State     string `json:"state"`
	TimeoutMs int    `json:"timeoutMs"`
}

// waitFor runs one wait.
func (c *Client) waitFor(raw json.RawMessage) (string, error) {
	var a waitArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", err
		}
	}
	if a.UDID == "" {
		return "", fmt.Errorf("udid is required")
	}
	timeout := waitTimeout(a.TimeoutMs)
	switch a.State {
	case "settled":
		return c.waitSettled(a)
	case "", "visible":
		return c.waitUntil(a, true, timeout)
	case "gone":
		return c.waitUntil(a, false, timeout)
	}
	return "", fmt.Errorf("state must be visible, gone or settled")
}

// waitTimeout clamps a requested timeout, defaulting when none was given.
func waitTimeout(ms int) time.Duration {
	if ms <= 0 {
		return waitDefault
	}
	return min(time.Duration(ms)*time.Millisecond, waitMax)
}

// waitUntil polls until the element is present (want true) or absent.
func (c *Client) waitUntil(a waitArgs, want bool, timeout time.Duration) (string, error) {
	if a.Ref == "" && a.Testid == "" && a.Text == "" && a.Role == "" && a.Name == "" {
		return "", errNoTarget
	}
	deadline := time.Now().Add(timeout)
	for {
		nodes, err := c.fetchTree(a.UDID, a.App)
		if err != nil {
			return "", err
		}
		present, err := c.present(nodes, a.deviceArgs)
		if err != nil {
			return "", err
		}
		if present == want {
			return fmt.Sprintf("%s is %s", targetName(a.deviceArgs), stateWord(want)), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for %s to be %s", timeout, targetName(a.deviceArgs), stateWord(want))
		}
		time.Sleep(waitPoll)
	}
}

func stateWord(visible bool) string {
	if visible {
		return "visible"
	}
	return "gone"
}

// present reports whether any element on screen matches the selector — any
// number, unlike tap, which needs exactly one.
func (c *Client) present(nodes []treeNode, a deviceArgs) (bool, error) {
	if a.Ref != "" {
		_, err := c.refIndex(nodes, a.UDID, a.Ref)
		if err != nil && strings.Contains(err.Error(), "unknown ref") {
			return false, err
		}
		return err == nil, nil
	}
	for _, n := range nodes {
		if matchesWait(n, a) {
			return true, nil
		}
	}
	return false, nil
}

// matchesWait is tap's selector rules without the uniqueness requirement.
func matchesWait(n treeNode, a deviceArgs) bool {
	switch {
	case a.Testid != "":
		return n.Identifier == a.Testid
	case a.Text != "":
		return nodeText(n, a.Text)
	}
	return roleAndName(n, a.Role, a.Name)
}

// waitSettled holds until the screen has stopped changing, through the
// server's settle barrier (the tree endpoint with ?after=).
func (c *Client) waitSettled(a waitArgs) (string, error) {
	path := "/api/devices/" + url.PathEscape(a.UDID) + "/tree?after="
	if a.App != "" {
		path += "&app=" + url.QueryEscape(a.App)
	}
	if _, err := c.get(path); err != nil {
		return "", unreadable(err)
	}
	return "screen settled", nil
}

// errUnreadable marks a failure to read the screen at all, which is not the
// same as an element being absent — the device may be busy, restarting its
// engine, or gone.
var errUnreadable = errors.New("could not read the device's screen")

// unreadable wraps a tree-read failure so an agent does not mistake it for
// "not found".
func unreadable(err error) error {
	return fmt.Errorf("%w (%w) — the device may be busy or restarting; this is not the element being absent, try again", errUnreadable, err)
}
