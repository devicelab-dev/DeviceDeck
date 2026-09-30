package sim

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordRun records the command it is given and fails when fail is set.
func recordRun(got *string, fail bool) runFunc {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*got = name + " " + strings.Join(args, " ")
		if fail {
			return nil, errors.New("exit status 1")
		}
		return nil, nil
	}
}

func TestSettingsCommands(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*Client) error
		want string
	}{
		{"dark", func(c *Client) error { return c.SetAppearance(ctx, "U", true) }, "xcrun simctl ui U appearance dark"},
		{"light", func(c *Client) error { return c.SetAppearance(ctx, "U", false) }, "xcrun simctl ui U appearance light"},
		{"location", func(c *Client) error { return c.SetLocation(ctx, "U", 37.3349, -122.009) }, "xcrun simctl location U set 37.3349,-122.009"},
		{"grant", func(c *Client) error { return c.SetPermission(ctx, "U", "com.x", "photos", true) }, "xcrun simctl privacy U grant photos com.x"},
		{"revoke", func(c *Client) error { return c.SetPermission(ctx, "U", "com.x", "location", false) }, "xcrun simctl privacy U revoke location com.x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := tt.call(&Client{run: recordRun(&got, false)}); err != nil || got != tt.want {
				t.Errorf("ran %q, err %v; want %q", got, err, tt.want)
			}
			if err := tt.call(&Client{run: recordRun(&got, true)}); err == nil || !strings.Contains(err.Error(), "U") {
				t.Errorf("failure not reported with the device: %v", err)
			}
		})
	}
}

func TestSetPermissionUnknownName(t *testing.T) {
	var got string
	err := (&Client{run: recordRun(&got, false)}).SetPermission(context.Background(), "U", "com.x", "camera", true)
	if err == nil || !strings.Contains(err.Error(), "calendar, contacts, location") || got != "" {
		t.Errorf("want a refusal listing the names and no command, got %v (ran %q)", err, got)
	}
}

func TestWaitBooted(t *testing.T) {
	var got string
	if err := (&Client{run: recordRun(&got, false)}).WaitBooted(context.Background(), "U"); err != nil || got != "xcrun simctl bootstatus U -b" {
		t.Errorf("ran %q, err %v", got, err)
	}
	if err := (&Client{run: recordRun(&got, true)}).WaitBooted(context.Background(), "U"); err == nil || !strings.Contains(err.Error(), "finish booting") {
		t.Errorf("failure not reported: %v", err)
	}
}
