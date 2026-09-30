package sim

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Device settings a test sets instead of tapping through the Settings app:
// appearance, a simulated location, and an app's permissions. They are the
// states app bugs hide behind (a dark-mode contrast bug, a screen that only
// renders with a location, a denied-permission path nobody walks by hand).

// iosPermissions maps DeviceDeck's permission names, shared with Android,
// to simctl privacy services. The simulator has no camera, so there is no
// camera permission to set; notifications are not a simctl privacy service.
var iosPermissions = map[string]string{
	"location":   "location",
	"microphone": "microphone",
	"contacts":   "contacts",
	"photos":     "photos",
	"calendar":   "calendar",
	"reminders":  "reminders",
	"motion":     "motion",
}

// SetAppearance switches the simulator between dark and light mode.
func (c *Client) SetAppearance(ctx context.Context, udid string, dark bool) error {
	style := "light"
	if dark {
		style = "dark"
	}
	if _, err := c.run(ctx, "xcrun", "simctl", "ui", udid, "appearance", style); err != nil {
		return fmt.Errorf("set %s appearance on %s: %w", style, udid, err)
	}
	return nil
}

// SetLocation fixes the simulator's reported location, which every app on
// it then sees through Core Location.
func (c *Client) SetLocation(ctx context.Context, udid string, lat, lon float64) error {
	point := fmt.Sprintf("%g,%g", lat, lon)
	if _, err := c.run(ctx, "xcrun", "simctl", "location", udid, "set", point); err != nil {
		return fmt.Errorf("set location %s on %s: %w", point, udid, err)
	}
	return nil
}

// SetPermission grants or revokes one permission for an app without the
// system prompt. iOS terminates the app when some permissions change, so a
// test sets them before launching it.
func (c *Client) SetPermission(ctx context.Context, udid, appID, permission string, grant bool) error {
	service, ok := iosPermissions[permission]
	if !ok {
		return fmt.Errorf("permission %q cannot be set on an iOS simulator; one of: %s", permission, PermissionNames(iosPermissions))
	}
	action := "revoke"
	if grant {
		action = "grant"
	}
	if _, err := c.run(ctx, "xcrun", "simctl", "privacy", udid, action, service, appID); err != nil {
		return fmt.Errorf("%s %s for %s on %s: %w", action, permission, appID, udid, err)
	}
	return nil
}

// PermissionNames lists a platform's permission names, sorted, for errors
// that tell the caller what it could have asked for.
func PermissionNames[V any](m map[string]V) string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
