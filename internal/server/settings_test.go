package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSettings records each settings call and fails the one named in fail.
type fakeSettings struct {
	calls []string
	fail  string
}

func (f *fakeSettings) record(call string) error {
	f.calls = append(f.calls, call)
	if f.fail != "" && strings.HasPrefix(call, f.fail) {
		return errors.New("simctl failed")
	}
	return nil
}

func (f *fakeSettings) SetAppearance(_ context.Context, udid string, dark bool) error {
	return f.record(fmt.Sprintf("%s appearance dark=%v", udid, dark))
}

func (f *fakeSettings) SetLocation(_ context.Context, udid string, lat, lon float64) error {
	return f.record(fmt.Sprintf("%s location %g,%g", udid, lat, lon))
}

func (f *fakeSettings) SetPermission(_ context.Context, udid, app, name string, grant bool) error {
	return f.record(fmt.Sprintf("%s permission %s %s grant=%v", udid, app, name, grant))
}

func TestSettingsApplied(t *testing.T) {
	fs := &fakeSettings{}
	s := newTestServer(&fakeBackend{})
	s.SetSettings(fs)
	rec := do(t, s, "POST", "/api/devices/AAA/settings",
		`{"appearance":"dark","location":{"lat":51.5,"lon":-0.12},"app":"com.x","grant":["camera"],"revoke":["location"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	want := []string{"AAA appearance dark=true", "AAA location 51.5,-0.12", "AAA permission com.x camera grant=true", "AAA permission com.x location grant=false"}
	if strings.Join(fs.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v, want %v", fs.calls, want)
	}
	if c := s.capture.(*fakeCapture); len(c.locations) != 1 || c.locations[0] != "AAA 51.5,-0.12" {
		t.Errorf("location not offered to the recording: %v", c.locations)
	}
	if !strings.Contains(rec.Body.String(), `"applied":["appearance dark","location 51.5,-0.12","grant camera","revoke location"]`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestSettingsRefused(t *testing.T) {
	tests := []struct {
		name, body, want string
		status           int
	}{
		{"bad appearance", `{"appearance":"sepia"}`, "dark", 400},
		{"bad latitude", `{"location":{"lat":91,"lon":0}}`, "lat in", 400},
		{"bad longitude", `{"location":{"lat":0,"lon":-181}}`, "lat in", 400},
		{"grant without app", `{"grant":["camera"]}`, "need app", 400},
		{"empty", `{}`, "nothing to set", 400},
		{"malformed", `{bad`, "", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSettings{}
			s := newTestServer(&fakeBackend{})
			s.SetSettings(fs)
			rec := do(t, s, "POST", "/api/devices/AAA/settings", tt.body)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.want) || len(fs.calls) != 0 {
				t.Errorf("status %d body %s calls %v", rec.Code, rec.Body, fs.calls)
			}
		})
	}
}

// A failure part-way reports what was already applied, since it stays.
func TestSettingsPartialFailure(t *testing.T) {
	fs := &fakeSettings{fail: "AAA permission"}
	s := newTestServer(&fakeBackend{})
	s.SetSettings(fs)
	rec := do(t, s, "POST", "/api/devices/AAA/settings", `{"appearance":"light","app":"com.x","grant":["camera"]}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "already applied: appearance light") {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}
	// A location the device refused is not recorded.
	fs = &fakeSettings{fail: "AAA location"}
	s.SetSettings(fs)
	if rec = do(t, s, "POST", "/api/devices/AAA/settings", `{"location":{"lat":1,"lon":2}}`); rec.Code != http.StatusBadGateway || len(s.capture.(*fakeCapture).locations) != 0 {
		t.Errorf("failed location: status %d, recorded %v", rec.Code, s.capture.(*fakeCapture).locations)
	}
	fs = &fakeSettings{fail: "AAA appearance"}
	s.SetSettings(fs)
	rec = do(t, s, "POST", "/api/devices/AAA/settings", `{"appearance":"light"}`)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "already applied") {
		t.Errorf("first-step failure: status %d body %s", rec.Code, rec.Body)
	}
}

func TestSettingsUnavailable(t *testing.T) {
	rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/settings", `{"appearance":"dark"}`)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status %d", rec.Code)
	}
}

func TestSettingsRouter(t *testing.T) {
	ios, android := &fakeSettings{}, &fakeSettings{}
	r := SettingsRouter{IOS: ios, Android: android}
	ctx := context.Background()
	_ = r.SetAppearance(ctx, "emulator-5554", true)
	_ = r.SetLocation(ctx, "EB69B42A-4763", 1, 2)
	_ = r.SetPermission(ctx, "emulator-5554", "com.x", "camera", true)
	if len(android.calls) != 2 || len(ios.calls) != 1 || !strings.HasPrefix(ios.calls[0], "EB69B42A-4763 location") {
		t.Errorf("routing wrong: ios=%v android=%v", ios.calls, android.calls)
	}
}

// ctxSettings records whether the context it was given was already done.
type ctxSettings struct {
	fakeSettings
	cancelled bool
}

func (c *ctxSettings) SetAppearance(ctx context.Context, udid string, dark bool) error {
	c.cancelled = ctx.Err() != nil
	return nil
}

// A client that goes away must not cancel a command that changes the
// device: simctl killed halfway leaves a half-applied setting.
func TestDeviceCommandsOutliveTheClient(t *testing.T) {
	cs := &ctxSettings{}
	s := newTestServer(&fakeBackend{})
	s.SetSettings(cs)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/api/devices/AAA/settings", strings.NewReader(`{"appearance":"dark"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || cs.cancelled {
		t.Errorf("status %d, command saw a cancelled context: %v", rec.Code, cs.cancelled)
	}
	opCtx, done := deviceOp(req)
	defer done()
	if _, ok := opCtx.Deadline(); !ok || opCtx.Err() != nil {
		t.Error("deviceOp must be live and bounded")
	}
}

// A launch starts the UI engine first and stops there when it cannot, before
// the app is touched: an agent started after the launch would push the app
// into the background.
func TestLaunchStartsEngineFirst(t *testing.T) {
	f := &fakeBackend{nodesErr: errors.New("agent did not start")}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/app/launch", `{"app":"com.x"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "UI engine") || len(f.launched) != 0 || len(f.reset) != 0 {
		t.Errorf("status %d body %s launched %v reset %v", rec.Code, rec.Body, f.launched, f.reset)
	}
}

// A device still booting is waited for before anything is installed or
// launched; one that never finishes fails the call without touching the app.
func TestLaunchAndInstallWaitForBoot(t *testing.T) {
	f := &fakeBackend{bootWaitErr: errors.New("android on emulator-5554 did not finish booting")}
	s := newTestServer(f)
	rec := do(t, s, "POST", "/api/devices/emulator-5554/app/launch", `{"app":"com.x"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "finish booting") || len(f.launched) != 0 {
		t.Errorf("launch: status %d body %s launched %v", rec.Code, rec.Body, f.launched)
	}
	rec = do(t, s, "POST", "/api/devices/emulator-5554/app/install", `{"appFile":"/x.apk"}`)
	if rec.Code != http.StatusBadGateway || len(f.installed) != 0 {
		t.Errorf("install: status %d body %s installed %v", rec.Code, rec.Body, f.installed)
	}
}
