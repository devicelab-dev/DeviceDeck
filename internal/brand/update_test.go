package brand

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatest(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"latest_version":"0.2.0"}`))
		case "/garbage":
			_, _ = w.Write([]byte(`not json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if v, err := Latest(ctx, srv.Client(), srv.URL+"/ok"); err != nil || v != "0.2.0" || gotUA != "devicedeck" {
		t.Fatalf("Latest = %q, %v (UA %q)", v, err, gotUA)
	}
	for _, path := range []string{"/missing", "/garbage"} {
		if _, err := Latest(ctx, srv.Client(), srv.URL+path); err == nil {
			t.Errorf("%s: expected an error", path)
		}
	}
	if _, err := Latest(ctx, srv.Client(), "http://127.0.0.1:1/unreachable"); err == nil {
		t.Error("unreachable server: expected an error")
	}
	if _, err := Latest(ctx, srv.Client(), "://bad-url"); err == nil {
		t.Error("bad URL: expected an error")
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"0.2.0", "0.1.0", true},
		{"v1.0.0", "0.9.9", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"0.2.0", "dev", false},
		{"0.2.0", "0.0.0-test", false},
		{"", "0.1.0", false},
		{"1.2", "1.1.0", false},
	} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestUpdateNotice(t *testing.T) {
	var b bytes.Buffer
	UpdateNotice(&b, "0.1.0", "0.2.0")
	if !strings.Contains(b.String(), "0.1.0 → 0.2.0") || !strings.Contains(b.String(), Install) {
		t.Errorf("notice = %q", b.String())
	}
}
