package brand

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLink(t *testing.T) {
	for _, tc := range []struct {
		url, text string
		hyper     bool
		want      string
	}{
		{Site, Maker, true, "\x1b]8;;https://devicelab.dev\x1b\\DeviceLab.dev\x1b]8;;\x1b\\"},
		{Site, Maker, false, "DeviceLab.dev (https://devicelab.dev)"},
		{Site, Site, false, "https://devicelab.dev"},
	} {
		if got := Link(tc.url, tc.text, tc.hyper); got != tc.want {
			t.Errorf("Link(%q, %q, %v) = %q, want %q", tc.url, tc.text, tc.hyper, got, tc.want)
		}
	}
}

func TestHyperlinks(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	t.Setenv("NO_COLOR", "")
	if Hyperlinks(file) {
		t.Error("a regular file is not a terminal")
	}
	closed, _ := os.Create(filepath.Join(t.TempDir(), "closed"))
	_ = closed.Close()
	if Hyperlinks(closed) {
		t.Error("a closed file cannot be a terminal")
	}
	t.Setenv("NO_COLOR", "1")
	if Hyperlinks(os.Stderr) {
		t.Error("NO_COLOR must turn hyperlinks off")
	}
}

func TestHyperlinksOnATerminal(t *testing.T) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		t.Skip("no controlling terminal in this environment")
	}
	defer func() { _ = tty.Close() }()
	t.Setenv("NO_COLOR", "")
	if !Hyperlinks(tty) {
		t.Error("a terminal should get hyperlinks")
	}
}

func TestBannerAndFooter(t *testing.T) {
	var b bytes.Buffer
	Banner(&b, "devicedeck 0.1.0 (abc1234)", false)
	Footer(&b, false)
	out := b.String()
	for _, want := range []string{
		"devicedeck 0.1.0 (abc1234) - by DeviceLab.dev (https://devicelab.dev)",
		"Automate your iOS and Android app like a web app.",
		"Star us on GitHub (" + Repo + ")",
		"Built by DeviceLab.dev (https://devicelab.dev) - Turn Your Devices Into a Distributed Device Lab",
		RealRuns + ": https://devicelab.dev",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("plain output carries escape codes: %q", out)
	}
}

func TestBannerOnATerminalIsStyled(t *testing.T) {
	var b bytes.Buffer
	Banner(&b, "devicedeck 0.1.0", true)
	Footer(&b, true)
	out := b.String()
	for _, want := range []string{
		"\x1b[1mdevicedeck 0.1.0\x1b[0m",                                  // bold product line
		"\x1b]8;;https://devicelab.dev\x1b\\\x1b[36mDeviceLab.dev\x1b[0m", // cyan, clickable maker
		"\x1b]8;;" + Repo + "\x1b\\Star us on GitHub",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("styled output missing %q:\n%q", want, out)
		}
	}
}

func TestFlowHeaderIsOnlyComments(t *testing.T) {
	h := FlowHeader()
	for _, line := range strings.Split(strings.TrimSuffix(h, "\n"), "\n") {
		if !strings.HasPrefix(line, "# ") {
			t.Errorf("flow header line is not a YAML comment: %q", line)
		}
	}
	if !strings.Contains(h, Site) || !strings.Contains(h, Repo) {
		t.Errorf("flow header lacks the links: %q", h)
	}
}
