// Package brand is the one place DeviceDeck says who makes it and where to
// go next: the terminal banner and footer, the header comment on every
// captured flow, and the update notice. Keeping the words and links here
// means every surface says the same thing, and changing a URL is one edit.
//
// The pattern and wording follow maestro-runner's: plain links (no tracking),
// shown to everyone, never gating anything. On a terminal, DeviceLab.dev is
// cyan and links are clickable (OSC 8); NO_COLOR or a pipe gets plain text
// with the address written out.
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
	Tagline  = "Automate your iOS and Android app like a web app."
	Mission  = "Turn Your Devices Into a Distributed Device Lab" // DeviceLab's line, as in maestro-runner
	RealRuns = "Run your captured flows unchanged on real devices"
)

// ANSI styles, applied only when the terminal takes them.
const (
	cyan  = "\x1b[36m"
	green = "\x1b[32m"
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

// Hyperlinks reports whether f is a terminal that should get colour and
// clickable links: a character device, and NO_COLOR unset (the convention
// for "plain output, please").

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

// style wraps text in an ANSI style when the terminal takes colour.
func style(code, text string, fancy bool) string {
	if !fancy {
		return text
	}
	return code + text + reset
}

// Bold and Cyan style text for a terminal, and return it untouched when the
// terminal does not take colour. Shared so every DeviceDeck message that
// highlights something highlights it the same way.
func Bold(text string, fancy bool) string { return style(bold, text, fancy) }

// Cyan: see Bold.
func Cyan(text string, fancy bool) string { return style(cyan, text, fancy) }

// Green: see Bold.
func Green(text string, fancy bool) string { return style(green, text, fancy) }

// Dim: see Bold. For secondary text, so the eye lands on what matters.
func Dim(text string, fancy bool) string { return style(dim, text, fancy) }

// maker is "DeviceLab.dev": cyan and clickable on a terminal, as in
// maestro-runner.
func maker(fancy bool) string {
	return Link(Site, style(cyan, Maker, fancy), fancy)
}

// Banner is printed when the server starts: who makes it on the first line,
// the pitch dimmed beneath.
func Banner(w io.Writer, versionLine string, fancy bool) {
	_, _ = fmt.Fprintf(w, "\n  %s  ·  by %s  ·  %s\n  %s\n",
		style(bold, versionLine, fancy), maker(fancy), Link(Repo, "★ Star us on GitHub", fancy),
		Dim(Tagline, fancy))
}

// Footer is printed when the server stops: DeviceLab's own line, as in
// maestro-runner, then where captured flows go next.
func Footer(w io.Writer, fancy bool) {
	_, _ = fmt.Fprintf(w, "\n  Built by %s - %s\n  %s: %s\n\n",
		maker(fancy), Mission, RealRuns, Link(Site, style(cyan, Site, fancy), fancy))
}

// FlowHeader is the comment block at the top of every captured flow. Maestro
// ignores comments, so the flow still runs byte-for-byte unchanged.
func FlowHeader() string {
	return strings.Join([]string{
		"# Captured with DeviceDeck by " + Maker + " - " + Repo,
		"# " + RealRuns + ": " + Site,
	}, "\n") + "\n"
}
