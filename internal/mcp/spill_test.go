package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/home"
)

func TestSpillJSON(t *testing.T) {
	c := &Client{OutDir: t.TempDir()}
	if out, err := c.spillJSON("ui_tree", `{"small":true}`); err != nil || out != `{"small":true}` {
		t.Errorf("small = %q %v", out, err)
	}
	big := `{"nodes":"` + strings.Repeat("x", maxToolText) + `"}`
	out, err := c.spillJSON("ui_tree", big)
	if err != nil || !strings.Contains(out, "too large") || !strings.Contains(out, "snapshot") {
		t.Fatalf("big = %.200q %v", out, err)
	}
	files, _ := filepath.Glob(filepath.Join(c.OutDir, "ui_tree-*.json"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	saved, _ := os.ReadFile(files[0])
	if string(saved) != big || !strings.Contains(out, files[0]) {
		t.Error("the full result was not kept, or its path not returned")
	}
	if info, _ := os.Stat(files[0]); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v", info.Mode().Perm())
	}
}

func TestCapLines(t *testing.T) {
	c := &Client{OutDir: t.TempDir()}
	line := strings.Repeat("y", 99) + "\n"
	text := strings.Repeat(line, maxToolText/100+50)
	out, err := c.capLines("snapshot", text)
	if err != nil || len(out) > maxToolText+300 || !strings.Contains(out, "more lines not shown") {
		t.Fatalf("capped = %d chars %v", len(out), err)
	}
	if !strings.HasPrefix(out, line) || strings.Contains(out[:strings.Index(out, "…")], "yy\ny") && !strings.HasSuffix(out[:strings.Index(out, "…")], "\n") {
		t.Error("not cut at a line boundary")
	}
	if small, _ := c.capLines("snapshot", "e1 button \"A\""); small != "e1 button \"A\"" {
		t.Errorf("small = %q", small)
	}
	// One huge line has no boundary to cut at: it is cut at the limit.
	one, err := c.capLines("snapshot", strings.Repeat("z", maxToolText+10))
	if err != nil || !strings.Contains(one, "0 more lines") {
		t.Errorf("single line: %.80q %v", one, err)
	}
}

func TestWriteOutFailures(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{OutDir: filepath.Join(blocked, "sub")} // parent is a file
	if _, err := c.spillJSON("ui_tree", strings.Repeat("x", maxToolText+1)); err == nil {
		t.Error("spill into an impossible folder must fail")
	}
	if _, err := c.capLines("snapshot", strings.Repeat("x\n", maxToolText)); err == nil {
		t.Error("cap into an impossible folder must fail")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := (&Client{OutDir: dir}).writeOut("x", "txt", "y"); err == nil {
		t.Error("write into a read-only folder must fail")
	}
}

func TestOutDir(t *testing.T) {
	t.Setenv(home.EnvHome, "/opt/dd")
	if got := outDir(); got != "/opt/dd/mcp-out" {
		t.Errorf("outDir = %q", got)
	}
	t.Setenv(home.EnvHome, "")
	t.Setenv("HOME", "")
	if got := outDir(); !strings.HasSuffix(got, "devicedeck-mcp-out") {
		t.Errorf("fallback = %q", got)
	}
}
