package main

import (
	"flag"
	"os"

	"github.com/devicelab-dev/DeviceDeck/internal/mcp"
)

// runMCP starts the stdio Model Context Protocol server — a thin adapter that
// gives an MCP client (Claude, Cursor, any agent) the same device tools a
// running `devicedeck serve` exposes over HTTP. It speaks JSON-RPC on
// stdin/stdout, the transport MCP clients spawn a server with.
//
// Coverage waiver: runMCP is process wiring — it parses flags and hands
// stdin/stdout to the server. The protocol and tools are unit-tested in
// internal/mcp.
func runMCP(args []string) error {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	server := flags.String("server", mcpServerURL(),
		"base URL of the running `devicedeck serve` to drive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	client := mcp.NewClient(*server)
	tools, order := client.Tools()
	return mcp.NewServer(tools, order).Serve(os.Stdin, os.Stdout)
}

// mcpServerURL defaults the target server to DEVICEDECK_URL, then to the
// serve default, so an MCP client configured with just `devicedeck mcp`
// finds a locally running server.
func mcpServerURL() string {
	if u := os.Getenv("DEVICEDECK_URL"); u != "" {
		return u
	}
	return "http://127.0.0.1:8787"
}
