package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	guarded := requireToken("s3cret", ok)
	tests := []struct {
		name       string
		prepare    func(r *http.Request)
		url        string
		wantStatus int
		wantCookie bool
	}{
		{"no token", func(*http.Request) {}, "/api/devices", http.StatusUnauthorized, false},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer s3cret") }, "/api/devices", http.StatusTeapot, false},
		{"wrong bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, "/api/devices", http.StatusUnauthorized, false},
		{"not bearer scheme", func(r *http.Request) { r.Header.Set("Authorization", "Basic s3cret") }, "/api/devices", http.StatusUnauthorized, false},
		{"cookie", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: TokenCookie, Value: "s3cret"}) }, "/", http.StatusTeapot, false},
		{"wrong cookie", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: TokenCookie, Value: "x"}) }, "/", http.StatusUnauthorized, false},
		{"query sets cookie", func(*http.Request) {}, "/device/booted?token=s3cret", http.StatusTeapot, true},
		{"wrong query", func(*http.Request) {}, "/device/booted?token=x", http.StatusUnauthorized, false},
		{"empty query", func(*http.Request) {}, "/device/booted?token=", http.StatusUnauthorized, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.url, nil)
			tt.prepare(r)
			w := httptest.NewRecorder()
			guarded.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			set := w.Header().Get("Set-Cookie")
			if tt.wantCookie != strings.Contains(set, TokenCookie+"=s3cret") {
				t.Errorf("Set-Cookie = %q, want cookie %v", set, tt.wantCookie)
			}
			if tt.wantCookie && (!strings.Contains(set, "HttpOnly") || !strings.Contains(set, "SameSite=Lax")) {
				t.Errorf("cookie flags = %q", set)
			}
			if w.Code == http.StatusUnauthorized && !strings.Contains(w.Body.String(), "Authorization: Bearer") {
				t.Errorf("401 body does not say how to get in: %s", w.Body.String())
			}
		})
	}
}

// No token set: the server stays open, as by default.
func TestRequireTokenOffPassesThrough(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	w := httptest.NewRecorder()
	requireToken("", ok).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d", w.Code)
	}
}

// The token guards the whole server: API, device page and console alike.
func TestServerHandlerRequiresToken(t *testing.T) {
	backend := &fakeBackend{}
	s := New(backend, backend, backend, backend, backend, backend, &fakeVideo{}, &fakeCapture{})
	s.SetAccessToken("s3cret")
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("with token: %d %s", w.Code, w.Body.String())
	}
}
