package server

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLogs sends slog to a buffer at debug level for the test's duration.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestLogRequestsLevelsAndFields(t *testing.T) {
	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tree") && r.URL.Query().Get("fail") != "":
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/missing"):
			w.WriteHeader(http.StatusNotFound)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	for _, tc := range []struct {
		method, path, want string
	}{
		{"GET", "/api/devices/AAA/tree", "level=DEBUG"},
		{"POST", "/api/devices/AAA/tap", "level=DEBUG"},
		{"POST", "/api/devices/AAA/boot", "level=INFO"},
		{"POST", "/api/devices/AAA/capture/stop", "level=INFO"},
		{"GET", "/api/devices/AAA/tree?fail=1", "level=WARN"},
		{"GET", "/api/devices/AAA/missing", "level=DEBUG"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			buf := captureLogs(t)
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, tc.path, nil))
			line := buf.String()
			if !strings.Contains(line, tc.want) || !strings.Contains(line, "method="+tc.method) ||
				!strings.Contains(line, "bytes=2") || !strings.Contains(line, "took=") {
				t.Errorf("log line = %q, want %s with method, bytes and duration", line, tc.want)
			}
		})
	}
}

func TestLogRequestsSurvivesWebSocketHijack(t *testing.T) {
	buf := captureLogs(t)
	logged := logRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack through the recorder failed: %v", err)
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n\r\n")
		_ = rw.Flush()
		_ = conn.Close()
	}))
	// A hijacked handler outlives the server's Close, so wait for it (and
	// its log line) explicitly before reading the log.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		logged.ServeHTTP(w, r)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/devices/AAA/video")
	if err == nil {
		_ = resp.Body.Close()
	}
	<-done
	if !strings.Contains(buf.String(), "status=101") {
		t.Errorf("log = %q, want status=101", buf.String())
	}
}

// plainWriter implements only http.ResponseWriter: no Hijack, no Flush.
type plainWriter struct{ http.ResponseWriter }

func TestStatusRecorderWithoutOptionalInterfaces(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: plainWriter{httptest.NewRecorder()}, status: http.StatusOK}
	if _, _, err := rec.Hijack(); err == nil {
		t.Error("Hijack on a plain writer should fail")
	}
	rec.Flush() // must be a no-op, not a panic

	flushing := httptest.NewRecorder()
	(&statusRecorder{ResponseWriter: flushing}).Flush()
	if !flushing.Flushed {
		t.Error("Flush not forwarded to a flushing writer")
	}
}

func TestHTTPErrorIsLogged(t *testing.T) {
	buf := captureLogs(t)
	httpError(httptest.NewRecorder(), http.StatusNotFound, errors.New("no such device"))
	httpError(httptest.NewRecorder(), http.StatusBadGateway, errors.New("engine down"))
	log := buf.String()
	if !strings.Contains(log, "level=DEBUG msg=\"request failed\" status=404") ||
		!strings.Contains(log, "level=WARN msg=\"request failed\" status=502") {
		t.Errorf("a 4xx must be debug and a 5xx a warning:\n%s", log)
	}
}
