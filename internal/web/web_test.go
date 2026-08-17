package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestConsoleServedAtRoot(t *testing.T) {
	rec := get(t, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "DeviceDeck") {
		t.Fatalf("root: %d", rec.Code)
	}
}

func TestStaticAssets(t *testing.T) {
	for _, path := range []string{"/app.js", "/style.css", "/device.js"} {
		if rec := get(t, path); rec.Code != 200 {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

func TestDevicePage(t *testing.T) {
	rec := get(t, "/device/EB69B42A-4763-4A33-AF0F-CD233F721951")
	if rec.Code != 200 {
		t.Fatalf("device page: %d", rec.Code)
	}
	body := rec.Body.String()
	// The automation page: mirror container present, device.js wired.
	for _, want := range []string{`id="mirror"`, "device.js", `id="video"`} {
		if !strings.Contains(body, want) {
			t.Errorf("device page missing %q", want)
		}
	}
}
