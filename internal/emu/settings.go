package emu

import (
	"context"
	"fmt"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// androidPermissions maps DeviceDeck's permission names, shared with iOS, to
// the runtime permissions behind them. A name covers every permission an app
// might declare for it (fine and coarse location; photos before and after
// Android 13), because pm grant refuses one the app did not declare.
var androidPermissions = map[string][]string{
	"location":      {"ACCESS_FINE_LOCATION", "ACCESS_COARSE_LOCATION"},
	"camera":        {"CAMERA"},
	"microphone":    {"RECORD_AUDIO"},
	"contacts":      {"READ_CONTACTS", "WRITE_CONTACTS"},
	"photos":        {"READ_MEDIA_IMAGES", "READ_EXTERNAL_STORAGE"},
	"calendar":      {"READ_CALENDAR", "WRITE_CALENDAR"},
	"notifications": {"POST_NOTIFICATIONS"},
	"motion":        {"ACTIVITY_RECOGNITION"},
}

// SetAppearance switches the emulator's system night mode.
func (c *Client) SetAppearance(ctx context.Context, serial string, dark bool) error {
	mode := "no"
	if dark {
		mode = "yes"
	}
	if _, err := c.run(ctx, "adb", "-s", serial, "shell", "cmd", "uimode", "night", mode); err != nil {
		return fmt.Errorf("set night mode %s on %s: %w", mode, serial, err)
	}
	return nil
}

// SetLocation fixes the emulator's GPS position through its console. geo fix
// takes longitude first.
func (c *Client) SetLocation(ctx context.Context, serial string, lat, lon float64) error {
	if _, err := c.run(ctx, "adb", "-s", serial, "emu", "geo", "fix", fmt.Sprintf("%g", lon), fmt.Sprintf("%g", lat)); err != nil {
		return fmt.Errorf("set location %g,%g on %s: %w", lat, lon, serial, err)
	}
	return nil
}

// SetPermission grants or revokes one permission for an app without the
// system prompt, for each permission the name covers that the app declares.
// The declared list is read first because pm grant on a permission the app
// does not declare does nothing and still exits 0 (measured on Android 16),
// so success would be reported for a grant that never happened.
func (c *Client) SetPermission(ctx context.Context, serial, appID, permission string, grant bool) error {
	perms, ok := androidPermissions[permission]
	if !ok {
		return fmt.Errorf("permission %q cannot be set on an Android emulator; one of: %s", permission, sim.PermissionNames(androidPermissions))
	}
	if err := checkPackage(appID); err != nil {
		return err
	}
	declared, err := c.declaredPermissions(ctx, serial, appID)
	if err != nil {
		return err
	}
	action := "revoke"
	if grant {
		action = "grant"
	}
	applied, err := c.applyDeclared(ctx, serial, appID, action, perms, declared)
	if err != nil {
		return fmt.Errorf("%s %s for %s on %s: %w", action, permission, appID, serial, err)
	}
	if applied == 0 {
		return fmt.Errorf("%s does not declare %s, so there is nothing to %s (it needs %s in its manifest)",
			appID, permission, action, strings.Join(perms, " or "))
	}
	return nil
}

// applyDeclared runs pm grant (or revoke) for each of perms appID declares,
// returning how many it applied.
func (c *Client) applyDeclared(ctx context.Context, serial, appID, action string, perms []string, declared map[string]bool) (int, error) {
	applied := 0
	for _, p := range perms {
		if !declared["android.permission."+p] {
			continue
		}
		if _, err := c.run(ctx, "adb", "-s", serial, "shell", "pm", action, appID, "android.permission."+p); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// indent is the number of leading spaces on line.
func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// declaredPermissions is the set of permissions appID requests in its
// manifest, from dumpsys package; an app that is not installed has none,
// which is an error.
func (c *Client) declaredPermissions(ctx context.Context, serial, appID string) (map[string]bool, error) {
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "dumpsys", "package", appID)
	if err != nil {
		return nil, fmt.Errorf("read %s's permissions on %s: %w", appID, serial, err)
	}
	lines := strings.Split(string(out), "\n")
	header := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "requested permissions:" {
			header = i
			break
		}
	}
	if header < 0 {
		return nil, fmt.Errorf("%s is not installed on %s", appID, serial)
	}
	// Entries are indented under the header; the next section is not. An
	// entry may carry flags after a colon ("…: restricted=true").
	declared := map[string]bool{}
	for _, line := range lines[header+1:] {
		if indent(line) <= indent(lines[header]) {
			break
		}
		name, _, _ := strings.Cut(strings.TrimSpace(line), ":")
		declared[name] = true
	}
	return declared, nil
}
