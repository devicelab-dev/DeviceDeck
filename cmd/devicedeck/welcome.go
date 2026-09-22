package main

import (
	"context"
	"fmt"
	"io"
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

// write prints the whole message.
func (w welcome) write(out io.Writer) {
	var b strings.Builder
	w.header(&b)
	w.devices(&b)
	doctor.PrintProblems(&b, w.tools, w.fancy)
	w.usage(&b)
	_, _ = io.WriteString(out, b.String())
}

func (w welcome) header(b *strings.Builder) {
	fmt.Fprintf(b, "  %s\n\n", brand.Bold("DeviceDeck is running.", w.fancy))
	fmt.Fprintf(b, "  Console          %s\n", w.link(w.local))
	for _, u := range w.network {
		fmt.Fprintf(b, "  On your network  %s\n", w.link(u))
	}
	b.WriteString("\n")
}

func (w welcome) devices(b *strings.Builder) {
	if len(w.booted) == 0 {
		fmt.Fprintf(b, "  No device is booted yet (%d available). Open the console and pick one.\n\n", w.total)
		return
	}
	fmt.Fprintf(b, "  Booted devices (%d of %d available), each a page your tests and agents can drive:\n", len(w.booted), w.total)
	for _, d := range w.booted {
		fmt.Fprintf(b, "    %-28s %-12s %s\n", d.Name, d.OS, w.link(w.local+"/device/"+d.UDID))
	}
	fmt.Fprintf(b, "  With one device up, %s always points at it.\n\n", w.link(w.local+"/device/booted"))
}

func (w welcome) usage(b *strings.Builder) {
	fmt.Fprintf(b, "  Use it from Claude Code:\n    %s\n    %s\n", claudePlaywright, claudeSkills)
	fmt.Fprintf(b, "    then ask: \"open %s/device/booted and log in to my app\"\n", w.local)
	fmt.Fprintf(b, "    or give Claude device tools directly: %s\n\n", claudeMCP)
	fmt.Fprintf(b, "  Use it from tests: point Playwright or Cypress at %s/device/<udid>\n\n", w.local)
	fmt.Fprintf(b, "  Logs  %s\n  Stop  Ctrl-C\n\n", w.logs)
}

func (w welcome) link(url string) string {
	return brand.Link(url, brand.Cyan(url, w.fancy), w.fancy)
}
