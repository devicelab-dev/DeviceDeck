package server

import (
	"bufio"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// lifecycleSuffixes are the requests that change what a device is doing.
// They are logged at info, so the terminal shows the session's story; the
// high-rate ones (taps, keys, tree polls, static files) stay at debug, where
// the run's log file still records every one.
var lifecycleSuffixes = []string{
	"/boot", "/app/launch", "/app/install", "/openurl",
	"/capture/start", "/capture/stop", "/capture/assert",
}

// logRequests records every request's method, path, status, size and
// duration. Failures (4xx/5xx) are warnings wherever they happen.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		began := time.Now()
		next.ServeHTTP(rec, r)
		slog.Log(r.Context(), requestLevel(r, rec.status), "http",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"bytes", rec.bytes, "took", time.Since(began), "remote", r.RemoteAddr)
	})
}

// requestLevel picks how loudly a finished request is logged.
func requestLevel(r *http.Request, status int) slog.Level {
	if status >= http.StatusBadRequest {
		return slog.LevelWarn
	}
	for _, suffix := range lifecycleSuffixes {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, suffix) {
			return slog.LevelInfo
		}
	}
	return slog.LevelDebug
}

// statusRecorder captures the status and size a handler wrote. It must
// still let the video and input WebSockets take over the connection, so it
// forwards Hijack and Flush to the real writer.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader records the status before passing it on.
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Write counts the body bytes sent.
func (s *statusRecorder) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Hijack hands the connection to a WebSocket upgrade; the status is then
// 101 Switching Protocols for the log line.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer cannot be hijacked")
	}
	s.status = http.StatusSwitchingProtocols
	return hj.Hijack()
}

// Flush passes streaming output straight through when the writer supports it.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
