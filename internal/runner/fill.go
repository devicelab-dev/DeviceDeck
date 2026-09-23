package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// FillRequest names a text field and the value it must end up holding —
// what a browser's fill() means. The field is found by its identifier when
// the page knows that identifier to be unique on screen, and by its centre
// otherwise. X and Y are in the tree's own units: points on iOS, pixels on
// Android.
type FillRequest struct {
	// App is the app under test. The iOS runner is always told it: a
	// command naming the bundle it already targets takes the runner's fast
	// foreground guard, skipping two waitForExistence preambles that cost
	// about a second each (XCUITest polls existence once a second).
	App        string
	Identifier string
	X, Y       float64
	Text       string
	// PrevLen is how many characters the device last reported in the field.
	// It sizes the clear when the new value is empty.
	PrevLen int
}

// errFillUnsupported is returned by an engine that cannot set a field's
// value; the caller falls back to keystrokes.
var errFillUnsupported = errors.New("engine cannot fill fields")

// fillEngine is an engine that can set a field's value in one driver call
// and confirm it landed.
type fillEngine interface {
	Fill(ctx context.Context, req FillRequest) error
}

// Fill sets a field on udid to req.Text through the device's driver and
// returns once the driver reports the value in place. Doing it as one
// command, instead of replaying keystrokes, is what lets the page tell a
// browser test the action is done only when the device has it.
func (s *Engines) Fill(ctx context.Context, udid string, req FillRequest) error {
	e, err := s.engine(ctx, udid)
	if err != nil {
		return err
	}
	f, ok := e.(fillEngine)
	if !ok {
		return errFillUnsupported
	}
	return f.Fill(ctx, req)
}

// iOS clear-to-empty: the runner's replace mode returns before clearing
// when the new text is empty (its empty-text guard runs first), so an empty
// fill is a focus tap followed by deletes, sized to what the field held.
const (
	backspace    = "\u0008"
	clearMinimum = 24
	clearMargin  = 8
)

// Fill sets the field through the XCUITest runner's type command in
// replace mode: the runner focuses the field (by identifier, else at the
// point), clears it, types, reads the value back and repairs once, failing
// with TEXT_ENTRY_MISMATCH if it still differs. A secure field cannot be
// read back, so for one the runner's word is all there is.
func (e *Engine) Fill(ctx context.Context, req FillRequest) error {
	x, y := req.X, req.Y
	if req.Text == "" {
		return e.clearField(ctx, req)
	}
	cmd := dlios.Command{
		Command: dlios.CmdType, AppBundleID: req.App, Text: req.Text, TextEntryMode: "replace", X: &x, Y: &y,
	}
	if req.Identifier != "" {
		cmd.SelectorKey, cmd.SelectorValue = "id", req.Identifier
	}
	if _, err := e.client.Call(runnerContext(ctx), cmd); err != nil {
		return fmt.Errorf("fill field: %w", err)
	}
	return nil
}

// clearField empties a field: tap it, then delete more characters than it
// held.
func (e *Engine) clearField(ctx context.Context, req FillRequest) error {
	x, y := req.X, req.Y
	tap := dlios.Command{Command: dlios.CmdTap, AppBundleID: req.App, X: &x, Y: &y}
	if _, err := e.client.Call(runnerContext(ctx), tap); err != nil {
		return fmt.Errorf("focus field: %w", err)
	}
	n := max(req.PrevLen+clearMargin, clearMinimum)
	del := dlios.Command{Command: dlios.CmdType, AppBundleID: req.App, Text: strings.Repeat(backspace, n)}
	if _, err := e.client.Call(runnerContext(ctx), del); err != nil {
		return fmt.Errorf("clear field: %w", err)
	}
	return nil
}

// Fill sets the field through the Android driver: the field is found by
// resource-id when that is unique, else focused with a tap at its centre
// and taken as the focused element; it is cleared, given the text, and
// read back from a fresh lookup — the driver caches an element's text at
// find time, so the element in hand cannot confirm anything.
func (e *AndroidEngine) Fill(_ context.Context, req FillRequest) error {
	el, err := e.fieldFor(req, true)
	if err != nil {
		return err
	}
	if err := el.Clear(); err != nil {
		return fmt.Errorf("clear field: %w", err)
	}
	if req.Text != "" {
		if err := el.SendKeys(req.Text); err != nil {
			return fmt.Errorf("fill field: %w", err)
		}
	}
	again, err := e.fieldFor(req, false)
	if err != nil {
		return err
	}
	if got, _ := again.Text(); req.Text != "" && got != req.Text && !isMasked(got, req.Text) {
		return fmt.Errorf("fill field: device holds %q, want %q", got, req.Text)
	}
	return nil
}

// fieldFor finds the field a request names. Without an identifier the field
// is the focused element; focus says whether to tap it first — only the
// first lookup does, the read-back must not tap again.
func (e *AndroidEngine) fieldFor(req FillRequest, focus bool) (*uiautomator2.Element, error) {
	if req.Identifier != "" {
		sel := fmt.Sprintf(`new UiSelector().resourceIdMatches(".*%s$")`, regexpQuote(req.Identifier))
		el, err := e.adapter.FindElement("-android uiautomator", sel)
		if err != nil {
			return nil, fmt.Errorf("find field %q: %w", req.Identifier, err)
		}
		return el, nil
	}
	if focus {
		if err := e.adapter.Click(int(req.X), int(req.Y)); err != nil {
			return nil, fmt.Errorf("focus field: %w", err)
		}
	}
	el, err := e.adapter.ActiveElement()
	if err != nil {
		return nil, fmt.Errorf("focused field: %w", err)
	}
	return el, nil
}

// isMasked reports whether got is a password field's masked rendering of
// want: Android reports a secure field's contents as bullets.
func isMasked(got, want string) bool {
	return got != "" && len([]rune(got)) == len([]rune(want)) && strings.Trim(got, "•*") == ""
}

// regexpQuote escapes a resource-id for a UiSelector regex (Java syntax).
func regexpQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
