package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeTools is a minimal tool set for exercising the protocol without HTTP.
func fakeTools() (map[string]Tool, []string) {
	tools := map[string]Tool{
		"echo": {
			Description: "echo the message",
			InputSchema: schemaNone(),
			Call: func(args json.RawMessage) (string, error) {
				var a struct {
					Msg  string `json:"msg"`
					Fail bool   `json:"fail"`
				}
				_ = json.Unmarshal(args, &a)
				if a.Fail {
					return "", errors.New("tool blew up")
				}
				return "echo: " + a.Msg, nil
			},
		},
	}
	return tools, []string{"echo"}
}

// decode reads one JSON-RPC response line.
func decode(t *testing.T, line string) response {
	t.Helper()
	var r response
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("bad response line %q: %v", line, err)
	}
	return r
}

func TestServeInitializeAndList(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, // notification: no reply
		``, // blank line: skipped
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n")
	var out bytes.Buffer
	s := NewServer(fakeTools())
	if err := s.Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 replies (notification and blank get none), got %d: %q", len(lines), out.String())
	}
	init := decode(t, lines[0])
	res := init.Result.(map[string]any)
	if res["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	list := decode(t, lines[1]).Result.(map[string]any)
	tools := list["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "echo" {
		t.Errorf("tools/list = %v", tools)
	}
}

func TestCallTool(t *testing.T) {
	tests := []struct {
		name        string
		params      string
		wantErrCode int  // non-zero ⇒ expect a JSON-RPC error
		wantIsError bool // for a successful call, whether the tool result is an error
		wantText    string
	}{
		{name: "success", params: `{"name":"echo","arguments":{"msg":"hi"}}`, wantText: "echo: hi"},
		{name: "tool error", params: `{"name":"echo","arguments":{"fail":true}}`, wantIsError: true, wantText: "tool blew up"},
		{name: "unknown tool", params: `{"name":"nope","arguments":{}}`, wantErrCode: -32602},
		{name: "bad params", params: `["not an object"]`, wantErrCode: -32602},
	}
	s := NewServer(fakeTools())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request{Method: "tools/call", ID: json.RawMessage(`9`), Params: json.RawMessage(tt.params)}
			resp := s.dispatch(req)
			if tt.wantErrCode != 0 {
				if resp.Error == nil || resp.Error.Code != tt.wantErrCode {
					t.Fatalf("want error %d, got %+v", tt.wantErrCode, resp)
				}
				return
			}
			r := resp.Result.(map[string]any)
			if r["isError"] != tt.wantIsError {
				t.Errorf("isError = %v, want %v", r["isError"], tt.wantIsError)
			}
			text := r["content"].([]map[string]any)[0]["text"]
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestDispatchUnknownMethod(t *testing.T) {
	s := NewServer(fakeTools())
	resp := s.dispatch(request{Method: "does/notexist", ID: json.RawMessage(`3`)})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("want -32601, got %+v", resp)
	}
}

func TestHandleLineParseError(t *testing.T) {
	s := NewServer(fakeTools())
	resp, reply := s.handleLine([]byte(`{not json`))
	if !reply || resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("want parse error, got reply=%v resp=%+v", reply, resp)
	}
}

// errReader fails on Read, so Serve returns the scanner's error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestServeReadError(t *testing.T) {
	s := NewServer(fakeTools())
	if err := s.Serve(errReader{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want read error")
	}
}

// errWriter fails on Write, so Serve returns the encoder's error.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write boom") }

func TestServeWriteError(t *testing.T) {
	s := NewServer(fakeTools())
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if err := s.Serve(strings.NewReader(in), errWriter{}); err == nil {
		t.Fatal("want write error")
	}
}
