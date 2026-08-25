// Package mcp is a Model Context Protocol server for DeviceDeck, spoken over
// stdio. It is a thin adapter: every tool is a small wrapper over the same
// HTTP API `devicedeck serve` already exposes, so an agent (Claude, Cursor,
// any MCP client) reaches simulators and emulators through the one core with
// no second implementation of anything. Keeping it in the binary — a
// `devicedeck mcp` subcommand, not a separate process — holds the
// single-binary guardrail (brief §11.3, §11.5: the MCP is a thin adapter,
// designed to cost a weekend, not a rework).
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// protocolVersion is the MCP revision this server implements. Sent back in
// the initialize result; a client that needs a different one negotiates on
// its side.
const protocolVersion = "2024-11-05"

// request is one JSON-RPC 2.0 message from the client. A request with no id
// is a notification and expects no reply.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is one JSON-RPC 2.0 reply. Exactly one of Result or Error is set.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC error object. Codes follow the spec: -32601 is an
// unknown method, -32602 bad params, -32603 an internal failure.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool is one callable the server exposes. Call receives the raw arguments
// object and returns the text an agent reads; an error becomes a tool-level
// failure (isError), not a transport error, so the model can react to it.
type Tool struct {
	Description string
	InputSchema map[string]any
	Call        func(args json.RawMessage) (string, error)
}

// Server routes MCP messages to a set of named tools. It holds no device
// state of its own — the tools reach the running server for that.
type Server struct {
	tools map[string]Tool
	order []string // registration order, so tools/list is stable
}

// NewServer returns a Server with the given tools. Registration order is
// preserved for a stable tools/list.
func NewServer(tools map[string]Tool, order []string) *Server {
	return &Server{tools: tools, order: order}
}

// Serve reads newline-delimited JSON-RPC from r and writes replies to w until
// r is exhausted. Notifications (no id) are handled without a reply. A line
// that will not parse gets a JSON-RPC parse error rather than ending the loop.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		resp, reply := s.handleLine(line)
		if !reply {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// handleLine parses and dispatches one message. reply is false for
// notifications and unparseable non-requests, which get no response.
func (s *Server) handleLine(line []byte) (response, bool) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, -32700, "parse error"), true
	}
	if len(req.ID) == 0 {
		return response{}, false // notification: initialized, cancelled, etc.
	}
	return s.dispatch(req), true
}

// dispatch routes a request by method to its result.
func (s *Server) dispatch(req request) response {
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, s.initializeResult())
	case "tools/list":
		return okResponse(req.ID, map[string]any{"tools": s.toolList()})
	case "tools/call":
		return s.callTool(req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("unknown method %q", req.Method))
	}
}

// initializeResult advertises the protocol version, the tools capability, and
// who this server is.
func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "devicedeck", "version": "0"},
	}
}

// toolList renders the registered tools in registration order for tools/list.
func (s *Server) toolList() []map[string]any {
	list := make([]map[string]any, 0, len(s.order))
	for _, name := range s.order {
		t := s.tools[name]
		list = append(list, map[string]any{
			"name":        name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return list
}

// callTool runs a tools/call request. A missing tool is a method-level error;
// a tool that returns an error is reported as an isError tool result so the
// model can read and react to it rather than the call failing at transport.
func (s *Server) callTool(req request) response {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, -32602, "invalid tools/call params")
	}
	tool, ok := s.tools[p.Name]
	if !ok {
		return errorResponse(req.ID, -32602, fmt.Sprintf("unknown tool %q", p.Name))
	}
	text, err := tool.Call(p.Arguments)
	if err != nil {
		return okResponse(req.ID, toolResult(err.Error(), true))
	}
	return okResponse(req.ID, toolResult(text, false))
}

// toolResult wraps text as an MCP tool result. isError marks a failure the
// model should see, not a transport error.
func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func okResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, msg string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
