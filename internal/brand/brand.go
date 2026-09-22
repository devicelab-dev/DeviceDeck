// Package brand is the one place DeviceDeck says who makes it and where to
// go next: the terminal banner and footer, the header comment on every
// captured flow, and the update notice. Keeping the words and links here
// means every surface says the same thing, and changing a URL is one edit.
//
// The pattern follows maestro-runner's: plain links (no tracking), shown to
// everyone, never gating anything. Terminal links are clickable (OSC 8)
// where the terminal supports it; NO_COLOR or a non-terminal gets plain text.
package brand

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Links and lines shared by every surface.
const (
	Site     = "https://devicelab.dev"
	Repo     = "https://github.com/devicelab-dev/DeviceDeck"
	Install  = "curl -fsSL https://open.devicelab.dev/install/devicedeck | bash"
	Maker    = "DeviceLab.dev"
	Tagline  = "Your simulators and emulators in a browser. Record a flow once, run it anywhere."
	RealRuns = "Run your captured flows unchanged on real devices"
)

// Hyperlinks reports whether f is a terminal that should get clickable
// links: a character device, and NO_COLOR unset (the convention for "plain
// output, please").
func Hyperlinks(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Link renders text as an OSC 8 terminal hyperlink to url, or as plain
// "text (url)" when hyperlinks are off, so the address is never lost.
func Link(url, text string, hyper bool) string {
	if !hyper {
		if text == url {
			return url
		}
		return text + " (" + url + ")"
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// Banner is printed when the server starts.
func Banner(w io.Writer, versionLine string, hyper bool) {
	_, _ = fmt.Fprintf(w, "\n  %s - by %s\n  %s\n  %s\n\n",
		versionLine, Link(Site, Maker, hyper), Tagline, Link(Repo, "Star us on GitHub", hyper))
}

// Footer is printed when the server stops.
func Footer(w io.Writer, hyper bool) {
	_, _ = fmt.Fprintf(w, "\n  Built by %s - %s: %s\n\n", Link(Site, Maker, hyper), RealRuns, Link(Site, Site, hyper))
}

// FlowHeader is the comment block at the top of every captured flow. Maestro
// ignores comments, so the flow still runs byte-for-byte unchanged.
func FlowHeader() string {
	return strings.Join([]string{
		"# Captured with DeviceDeck by " + Maker + " - " + Repo,
		"# " + RealRuns + ": " + Site,
	}, "\n") + "\n"
}
