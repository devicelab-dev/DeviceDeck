package main

import (
	"strings"
	"testing"
)

func TestMCPServerURL(t *testing.T) {
	t.Run("defaults when env unset", func(t *testing.T) {
		t.Setenv("DEVICEDECK_URL", "")
		if got := mcpServerURL(); got != "http://127.0.0.1:8787" {
			t.Errorf("mcpServerURL = %q, want the serve default", got)
		}
	})
	t.Run("honors DEVICEDECK_URL", func(t *testing.T) {
		t.Setenv("DEVICEDECK_URL", "http://host:9000")
		if got := mcpServerURL(); got != "http://host:9000" {
			t.Errorf("mcpServerURL = %q", got)
		}
	})
}

func TestUsageMentionsMCP(t *testing.T) {
	var b strings.Builder
	usage(&b)
	if !strings.Contains(b.String(), "devicedeck mcp") {
		t.Errorf("usage does not mention the mcp subcommand:\n%s", b.String())
	}
}
