package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Two devices driven through one MCP client, as when an agent runs two
// simulators side by side: every request must go to the device the call
// named, concurrent calls must not cross, and a ref minted on one device must
// not act on the other (other mobile MCP servers sent the second device's
// taps to the first).
func TestTwoDevicesStayIsolated(t *testing.T) {
	var mu sync.Mutex
	taps := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		udid := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")[0]
		switch {
		case strings.HasSuffix(r.URL.Path, "/tree"):
			label := map[string]string{"A": "Alpha", "B": "Beta"}[udid]
			_, _ = io.WriteString(w, tree(`{"index":1,"type":"Button","identifier":"go-`+udid+`","label":"`+label+`","frame":{"x":0,"y":0,"width":400,"height":80}}`))
		case strings.HasSuffix(r.URL.Path, "/tap"):
			mu.Lock()
			taps[udid]++
			mu.Unlock()
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	for _, d := range []string{"A", "B"} {
		if _, err := c.snapshot(raw(map[string]any{"udid": d})); err != nil {
			t.Fatalf("snapshot %s: %v", d, err)
		}
	}

	const rounds = 20
	var wg sync.WaitGroup
	for i := 0; i < rounds; i++ {
		for _, d := range []string{"A", "B"} {
			wg.Add(1)
			go func(d string) {
				defer wg.Done()
				if _, err := c.tap(raw(map[string]any{"udid": d, "testid": "go-" + d})); err != nil {
					t.Errorf("tap %s: %v", d, err)
				}
			}(d)
		}
	}
	wg.Wait()
	if taps["A"] != rounds || taps["B"] != rounds || len(taps) != 2 {
		t.Errorf("taps crossed devices: %v", taps)
	}

	// Device A's e1 is "Alpha"; on B, e1 names B's own element and a ref A
	// alone was given does not exist.
	if _, err := c.tap(raw(map[string]any{"udid": "B", "ref": "e2"})); err == nil || !strings.Contains(err.Error(), "unknown ref") {
		t.Errorf("a ref never issued on B must be refused, got %v", err)
	}
	if taps["B"] != rounds {
		t.Errorf("refused tap still reached the device: %v", taps)
	}
}
