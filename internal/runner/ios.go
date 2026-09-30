package runner

import (
	"context"
	"errors"
	"fmt"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
)

// agentCaller is the one call DeviceDeck makes on the devicelab iOS agent;
// satisfied by *dlios.Client and by test stand-ins.
type agentCaller interface {
	Call(ctx context.Context, cmd string, args *dlios.Args) (*dlios.Response, error)
}

// TreeClient reads UI snapshots from a running devicelab iOS agent.
type TreeClient struct {
	client agentCaller
}

// NewTreeClient targets an agent listening on 127.0.0.1:port — the agent
// only ever binds loopback.
func NewTreeClient(port int) *TreeClient {
	return &TreeClient{client: dlios.NewClient(port)}
}

// Snapshot returns the full UI tree for appBundleID (empty = the frontmost
// app, agent-side default).
func (t *TreeClient) Snapshot(ctx context.Context, appBundleID string) ([]Node, error) {
	snap, err := t.SnapshotState(ctx, appBundleID)
	return snap.Nodes, err
}

// SnapshotState is Snapshot plus the app's lifecycle state, read from the
// same capture. A snapshot never activates the app, so AppState is its
// true frontmost/background state, which the node tree cannot report
// reliably. An app that is not in front answers with its state and no tree.
// System alerts (permission prompts, which SpringBoard draws, not the app) are
// always included: the page shows them as an alertdialog a test can answer.
//
// A named app's state is asked first, and one not in front is not captured:
// the agent's first capture of a backgrounded app after it starts hangs past
// every timeout (measured on iOS 27: 60s, then the agent stops answering
// until restarted), where the state answers in a millisecond.
func (t *TreeClient) SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error) {
	if state := t.appState(ctx, appBundleID); state != "" && state != appForeground {
		return Snapshot{AppState: state}, nil
	}
	resp, err := t.client.Call(ctx, "snapshot", &dlios.Args{App: appBundleID, Alerts: true})
	if err != nil {
		if snap, ok := t.notFrontmost(ctx, appBundleID, err); ok {
			return snap, nil
		}
		return Snapshot{}, fmt.Errorf("agent snapshot: %w", err)
	}
	if resp == nil || resp.Data == nil {
		return Snapshot{}, errors.New("agent snapshot: empty response")
	}
	state := appStates[resp.Data.AppState]
	if state != "" && state != appForeground {
		return Snapshot{AppState: state}, nil
	}
	// Culled here rather than in the browser: the console, the device
	// page and Flow Capture all read this tree, and a flow recorded
	// against a screen nobody can see is the failure that survives to
	// real hardware.
	return Snapshot{Nodes: OnScreen(convertNodes(resp.Data.Nodes)), AppState: state}, nil
}

// notFrontmost explains a failed snapshot of a named app by asking the
// agent for that app's state again: an app that left the front between the
// state check and the capture cannot be captured, and that is a state to
// report, not an error. A frontmost app that cannot be read is still an error.
func (t *TreeClient) notFrontmost(ctx context.Context, app string, err error) (Snapshot, bool) {
	var agentErr *dlios.AgentError
	if app == "" || !errors.As(err, &agentErr) || agentErr.Code != dlios.ErrSnapshotFailed {
		return Snapshot{}, false
	}
	state := t.appState(ctx, app)
	if state == "" || state == appForeground {
		return Snapshot{}, false
	}
	return Snapshot{AppState: state}, true
}

// appState is a named app's lifecycle state from the agent, or "" when no
// app is named or the agent cannot tell.
func (t *TreeClient) appState(ctx context.Context, app string) string {
	if app == "" {
		return ""
	}
	resp, err := t.client.Call(ctx, "app", &dlios.Args{Action: "state", BundleID: app})
	if err != nil || resp == nil || resp.Data == nil {
		return ""
	}
	return appStates[resp.Data.AppState]
}

// appForeground is DeviceDeck's state for an app in front, taking input.
const appForeground = "runningForeground"

// appStates maps the agent's state names onto XCUIApplication's, the
// vocabulary DeviceDeck's API has always reported. Unknown names map to "",
// which callers treat as "cannot tell".
var appStates = map[string]string{
	"foreground": appForeground,
	"background": "runningBackground",
	"notRunning": "notRunning",
}

// idleCapMs bounds one idle wait. Measured: iOS keeps a pushed-away screen
// in the tree ~430ms after the push animation stops; an app that never
// idles (a spinner, a looping animation) costs at most this.
const idleCapMs = 1000

// Idle waits, up to idleCapMs, for the app to finish its work: the agent's
// settle, which waits for XCTest's app-idle quiescence and then for frames
// and tree to stop changing. The tree alone cannot tell a finished
// transition from one that has stopped moving but not yet removed the old
// screen.
func (t *TreeClient) Idle(ctx context.Context, appBundleID string) error {
	_, err := t.client.Call(ctx, "settle", &dlios.Args{App: appBundleID, TimeoutMs: idleCapMs, Quiescence: "all"})
	return err
}

// convertNodes turns the agent's flat node list into DeviceDeck's. Depth is
// recovered from the parent chain (the agent lists parents before their
// children), and an element counts as hittable when any of it is on screen:
// the agent reports visibility, not XCUITest's hittable, which DeviceDeck
// only ever used as a diagnostic. An element with a title but no label
// takes the title as its label: the runner matches text on the title too,
// so a flow captured against it replays.
func convertNodes(in []dlios.Node) []Node {
	out := make([]Node, len(in))
	for i, n := range in {
		out[i] = Node{
			Index:       n.I,
			Type:        n.Type,
			Label:       firstNonEmpty(n.Label, n.Title),
			Identifier:  n.ID,
			Value:       n.Value,
			Placeholder: n.Placeholder,
			Frame:       Rect{X: n.X, Y: n.Y, Width: n.W, Height: n.H},
			Enabled:     n.Enabled,
			Focused:     n.Focused,
			Selected:    n.Selected,
			Hittable:    n.Vis > 0,
		}
		if n.P >= 0 && n.P < i {
			parent := n.P
			out[i].ParentIndex = &parent
			out[i].Depth = out[parent].Depth + 1
		}
	}
	return out
}

// Engine owns the devicelab iOS agent for one simulator. The agent ships
// prebuilt; starting it costs seconds (or nothing, when a previous run left
// it running), so Engines are cached per UDID and live until Stop.
type Engine struct {
	tree  *TreeClient
	agent *dlios.Agent
	// client is the same agent client the tree reads through; Stop needs
	// the concrete type.
	client *dlios.Client
}

// StartEngine starts (or re-attaches to) the agent on udid.
//
// Coverage waiver: StartEngine and Engine.Stop wrap the simctl-driven agent
// lifecycle and only execute against a real booted simulator; they are
// exercised by end-to-end runs, not unit tests.
func StartEngine(ctx context.Context, udid string) (*Engine, error) {
	agent, client, err := dlios.StartAgent(ctx, dlios.AgentOptions{UDID: udid})
	if err != nil {
		return nil, fmt.Errorf("start devicelab iOS agent: %w", err)
	}
	return &Engine{tree: &TreeClient{client: client}, agent: agent, client: client}, nil
}

// Snapshot fetches the UI tree via the engine's agent.
func (e *Engine) Snapshot(ctx context.Context, appBundleID string) ([]Node, error) {
	return e.tree.Snapshot(ctx, appBundleID)
}

// SnapshotState fetches the UI tree and the app's lifecycle state.
func (e *Engine) SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error) {
	return e.tree.SnapshotState(ctx, appBundleID)
}

// Idle waits for the app to finish its work; see TreeClient.Idle.
func (e *Engine) Idle(ctx context.Context, appBundleID string) error {
	return e.tree.Idle(ctx, appBundleID)
}

// Release lets go of the agent: one started through simctl stays running
// as a daemon, so the next DeviceDeck start re-attaches to it instead of
// launching it again; one this process runs under xcodebuild cannot outlive
// it and is stopped (the driver decides).
//
// Coverage waiver: like Stop, it drives the real agent lifecycle.
func (e *Engine) Release(ctx context.Context) {
	e.agent.Release(ctx, e.client)
}

// Stop asks the agent to exit. The simulator itself is left running; the
// caller decides whether to power it off.
func (e *Engine) Stop(ctx context.Context) error {
	e.agent.Stop(ctx, e.client)
	return nil
}

// iOS clear: the agent deletes backwards from the caret, which a focus tap
// leaves at the end of the text; it deletes more than the field held.
const (
	clearMinimum = 24
	clearMargin  = 8
)

// Fill focuses the field with a tap at its centre, deletes what it held and
// types the new value, then checks the field's value as the agent reads it
// back. The agent verifies each keystroke batch and repairs lost keys
// itself; the final check here is that the field holds exactly req.Text,
// not merely ends with it. A secure field reads back as bullets, so for one
// a matching length is all there is to confirm.
func (e *Engine) Fill(ctx context.Context, req FillRequest) error {
	args := &dlios.Args{App: req.App, Erase: max(req.PrevLen+clearMargin, clearMinimum), Text: req.Text}
	resp, err := e.typeInto(ctx, req, args)
	if err != nil {
		return fmt.Errorf("fill field: %w", err)
	}
	if req.Text == "" || resp == nil || resp.Data == nil || resp.Data.Text == nil {
		return nil
	}
	if got := *resp.Data.Text; got != req.Text && !isMasked(got, req.Text) {
		return fmt.Errorf("fill field: device holds %q, want %q", got, req.Text)
	}
	return nil
}

// typeInto taps the field and types. When nothing took focus — a tap that
// landed while the field was still arriving — it taps once more and types
// again, the same recovery the driver's inputText makes.
func (e *Engine) typeInto(ctx context.Context, req FillRequest, args *dlios.Args) (*dlios.Response, error) {
	resp, err := e.tapAndType(ctx, req, args)
	if isNoFocus(err) {
		resp, err = e.tapAndType(ctx, req, args)
	}
	return resp, err
}

func (e *Engine) tapAndType(ctx context.Context, req FillRequest, args *dlios.Args) (*dlios.Response, error) {
	x, y := req.X, req.Y
	tap := &dlios.Args{Kind: "tap", X: &x, Y: &y, Settle: true}
	if _, err := e.tree.client.Call(ctx, "act", tap); err != nil {
		return nil, fmt.Errorf("focus: %w", err)
	}
	return e.tree.client.Call(ctx, "type", args)
}

// isNoFocus reports the agent's "no field has keyboard focus" answer.
func isNoFocus(err error) bool {
	var agentErr *dlios.AgentError
	return errors.As(err, &agentErr) && agentErr.Code == "NO_FOCUS"
}

// firstNonEmpty returns the first of values that is not empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
