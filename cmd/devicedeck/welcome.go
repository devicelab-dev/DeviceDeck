package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/brand"
	"github.com/devicelab-dev/DeviceDeck/internal/doctor"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// The Claude Code setup the README recommends, repeated where people look
// first: the terminal they just started DeviceDeck in.
const (
	claudePlaywright = "claude mcp add playwright npx @playwright/mcp@latest"
	claudeSkills     = "claude plugin marketplace add devicelab-dev/DeviceDeck"
	claudeMCP        = "claude mcp add devicedeck -- devicedeck mcp"
)

// welcome is what serve prints once it is listening, for the person who
// started it: where to open it, which devices are up with a direct link to
// each, and how to drive them from Claude Code or a test. The log file keeps
// the machine-oriented details.
type welcome struct {
	local   string
	network []string
	booted  []sim.Device
	total   int
	tools   []doctor.Result
	logs    string
	fancy   bool
}

// newWelcome gathers the device picture and checks the tools, side by side
// so the checks add no wait. A listing failure still prints the rest: the
// links and setup are what matter most at startup.
func newWelcome(ctx context.Context, devices server.DeviceLister, tools doctor.Env,
	local string, network []string, logs string, fancy bool,
) welcome {
	w := welcome{local: local, network: network, logs: logs, fancy: fancy}
	checked := make(chan []doctor.Result, 1)
	go func() { checked <- doctor.Run(ctx, tools) }()
	all, _ := devices.All(ctx)
	w.total = len(all)
	for _, d := range all {
		if d.Booted {
			w.booted = append(w.booted, d)
		}
	}
	sort.Slice(w.booted, func(i, j int) bool { return w.booted[i].Name < w.booted[j].Name })
	w.tools = <-checked
	return w
}

// write prints the whole message: a status line, then short sections with
// bold headings, links in cyan, commands on their own $ lines, and
// explanations dimmed so the eye goes to what can be clicked or copied.
func (w welcome) write(out io.Writer) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s %s\n", brand.Green("●", w.fancy), brand.Bold("Running", w.fancy))
	w.open(&b)
	w.devices(&b)
	w.toolsSection(&b)
	w.claude(&b)
	w.section(&b, "USE WITH TESTS")
	fmt.Fprintf(&b, "    %-10s %s\n", "baseURL", w.link(w.local+"/device/<udid>"))
	fmt.Fprintf(&b, "\n  %s  %s\n  %s  %s\n\n", brand.Dim("Logs", w.fancy), tildeHome(w.logs),
		brand.Dim("Stop", w.fancy), "Ctrl-C")
	_, _ = io.WriteString(out, b.String())
}

// section starts a block with a blank line and a bold heading.
func (w welcome) section(b *strings.Builder, title string) {
	fmt.Fprintf(b, "\n  %s\n", brand.Bold(title, w.fancy))
}

func (w welcome) open(b *strings.Builder) {
	w.section(b, "OPEN")
	fmt.Fprintf(b, "    %-10s %s\n", "Console", w.link(w.local))
	for i, u := range w.network {
		label := ""
		if i == 0 {
			label = "Network"
		}
		fmt.Fprintf(b, "    %-10s %s\n", label, w.link(u))
	}
}

func (w welcome) devices(b *strings.Builder) {
	w.section(b, "DEVICES")
	if len(w.booted) == 0 {
		fmt.Fprintf(b, "    None booted yet. %s\n",
			brand.Dim(fmt.Sprintf("Pick one of %d in the console.", w.total), w.fancy))
		return
	}
	for _, d := range w.booted {
		// Pad before styling: colour codes are invisible but counted.
		fmt.Fprintf(b, "    %-22s %s %s\n", d.Name, brand.Dim(fmt.Sprintf("%-10s", d.OS), w.fancy), w.link(w.local+"/device/"+d.UDID))
	}
	fmt.Fprintf(b, "    %s\n", brand.Dim(fmt.Sprintf("%d of %d booted · /device/booted opens the only one", len(w.booted), w.total), w.fancy))
}

func (w welcome) toolsSection(b *strings.Builder) {
	w.section(b, "TOOLS")
	problems := doctor.Problems(w.tools)
	if len(problems) == 0 {
		fmt.Fprintf(b, "    %s All %d found  %s\n", brand.Green("✓", w.fancy), len(w.tools),
			brand.Dim("devicedeck doctor for details", w.fancy))
		return
	}
	for _, r := range problems {
		b.WriteString(doctor.Line(r, w.fancy))
	}
}

func (w welcome) claude(b *strings.Builder) {
	w.section(b, "USE WITH CLAUDE CODE")
	steps := []struct{ what, do string }{
		{"Add the browser tool", w.cmd(claudePlaywright)},
		{"Add DeviceDeck's skills", w.cmd(claudeSkills)},
		{"Ask Claude", fmt.Sprintf("\"Open %s/device/booted and log in to my app\"", w.local)},
	}
	for i, s := range steps {
		fmt.Fprintf(b, "    %d. %s\n       %s\n", i+1, s.what, s.do)
	}
	fmt.Fprintf(b, "    %s\n       %s\n", brand.Dim("Or give Claude device tools over MCP:", w.fancy), w.cmd(claudeMCP))
}

// cmd shows a command to copy, after a dimmed $ prompt.
func (w welcome) cmd(c string) string { return brand.Dim("$", w.fancy) + " " + c }

func (w welcome) link(url string) string {
	return brand.Link(url, brand.Cyan(url, w.fancy), w.fancy)
}

// tildeHome shortens a path under the home folder to ~/..., the way people
// type it.
func tildeHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
