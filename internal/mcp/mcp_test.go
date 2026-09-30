package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
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
	if hint, _ := res["instructions"].(string); !strings.Contains(hint, "http://127.0.0.1:8787/device/{udid}") || !strings.Contains(hint, "data-testid") {
		t.Errorf("instructions do not tell the agent how to drive a device: %q", hint)
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

func TestCallToolRaw(t *testing.T) {
	tools := map[string]Tool{
		"shot": {
			Description: "raw image tool",
			InputSchema: schemaNone(),
			Raw: func(args json.RawMessage) ([]any, error) {
				var a struct {
					Fail bool `json:"fail"`
				}
				_ = json.Unmarshal(args, &a)
				if a.Fail {
					return nil, errors.New("no screen")
				}
				return []any{map[string]any{"type": "image", "data": "AAAA", "mimeType": "image/png"}}, nil
			},
		},
	}
	s := NewServer(tools, []string{"shot"})

	// Success: content is the raw items, isError false.
	ok := s.dispatch(request{
		Method: "tools/call", ID: json.RawMessage(`1`),
		Params: json.RawMessage(`{"name":"shot","arguments":{}}`),
	})
	r := ok.Result.(map[string]any)
	if r["isError"] != false {
		t.Errorf("raw success isError = %v", r["isError"])
	}
	if item := r["content"].([]any)[0].(map[string]any); item["type"] != "image" {
		t.Errorf("raw content = %v", item)
	}

	// Failure: reported as an isError tool result, not a transport error.
	bad := s.dispatch(request{
		Method: "tools/call", ID: json.RawMessage(`2`),
		Params: json.RawMessage(`{"name":"shot","arguments":{"fail":true}}`),
	})
	br := bad.Result.(map[string]any)
	if br["isError"] != true {
		t.Errorf("raw failure isError = %v", br["isError"])
	}
}

func TestToolCallsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	long := strings.Repeat("x", maxLoggedArgs+50)
	s := NewServer(fakeTools())
	for _, tc := range []struct {
		params, want string
	}{
		{`{"name":"echo","arguments":{"msg":"hi"}}`, `level=INFO msg="mcp tool" tool=echo`},
		{`{"name":"echo","arguments":{"fail":true}}`, `level=WARN msg="mcp tool" tool=echo`},
		{`{"name":"nope","arguments":{}}`, `level=WARN msg="mcp tool" tool=nope`},
		{`["not an object"]`, `level=WARN msg="mcp tool" tool=""`},
		{`{"name":"echo","arguments":{"msg":"` + long + `"}}`, `…"`},
	} {
		buf.Reset()
		s.dispatch(request{Method: "tools/call", ID: json.RawMessage(`1`), Params: json.RawMessage(tc.params)})
		if line := buf.String(); !strings.Contains(line, tc.want) || !strings.Contains(line, "took=") {
			t.Errorf("params %.40s: log = %q, want %q", tc.params, line, tc.want)
		}
	}
}

// The instructions name the server the tools actually drive, not the default.
func TestInstructionsFollowBaseURL(t *testing.T) {
	tools, order := fakeTools()
	s := NewServer(tools, order).WithBaseURL("http://10.0.0.5:9000/")
	if got := s.instructions(); !strings.Contains(got, "http://10.0.0.5:9000/device/{udid}") || strings.Contains(got, "8787") {
		t.Errorf("instructions = %q", got)
	}
}

// syncBuffer is a bytes.Buffer safe for Serve's concurrent writers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A slow tool call does not hold up the next one: "slow" waits until "fast"
// has run, which could never happen if calls were served one at a time.
func TestServeRunsToolCallsConcurrently(t *testing.T) {
	fastDone := make(chan struct{})
	tools := map[string]Tool{
		"slow": {InputSchema: schemaNone(), Call: func(json.RawMessage) (string, error) {
			select {
			case <-fastDone:
				return "slow done", nil
			case <-time.After(5 * time.Second):
				return "", errors.New("fast never ran: calls are serial")
			}
		}},
		"fast": {InputSchema: schemaNone(), Call: func(json.RawMessage) (string, error) {
			close(fastDone)
			return "fast done", nil
		}},
	}
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fast"}}` + "\n"
	var out syncBuffer
	if err := NewServer(tools, []string{"slow", "fast"}).Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "slow done") || !strings.Contains(got, "fast done") {
		t.Errorf("replies = %s", got)
	}
}

// A call that carries a progressToken gets progress notifications while it
// runs; one without does not.
func TestServeReportsProgress(t *testing.T) {
	prev := progressEvery
	progressEvery = 10 * time.Millisecond
	t.Cleanup(func() { progressEvery = prev })
	tools := map[string]Tool{"wait": {InputSchema: schemaNone(), Call: func(json.RawMessage) (string, error) {
		time.Sleep(60 * time.Millisecond)
		return "done", nil
	}}}
	s := NewServer(tools, []string{"wait"})
	var out syncBuffer
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"wait","_meta":{"progressToken":"tok-1"}}}` + "\n"
	if err := s.Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, `"method":"notifications/progress"`) || !strings.Contains(got, `"progressToken":"tok-1"`) {
		t.Errorf("no progress notification: %s", got)
	}
	var quiet syncBuffer
	in = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"wait"}}` + "\n"
	if err := s.Serve(strings.NewReader(in), &quiet); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(quiet.String(), "notifications/progress") {
		t.Errorf("progress sent without a token: %s", quiet.String())
	}
}

// Lines that are not tool calls are still answered in order: a parse error,
// a notification (no reply), and a tools/call with no id (a notification).
func TestServeRoutesNonCalls(t *testing.T) {
	in := "{not json\n" + `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"echo"}}` + "\n"
	var out syncBuffer
	if err := NewServer(fakeTools()).Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, "-32700") {
		t.Errorf("replies = %q", got)
	}
}
