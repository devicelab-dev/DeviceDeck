package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

// DeviceSettings sets the device states a test would otherwise reach through
// the Settings app: dark mode, a simulated location, and an app's
// permissions granted or revoked without the system prompt.
type DeviceSettings interface {
	SetAppearance(ctx context.Context, udid string, dark bool) error
	SetLocation(ctx context.Context, udid string, lat, lon float64) error
	SetPermission(ctx context.Context, udid, appID, permission string, grant bool) error
}

// SettingsRouter picks the platform's settings backend per device.
type SettingsRouter struct {
	IOS     DeviceSettings
	Android DeviceSettings
}

func (r SettingsRouter) pick(udid string) DeviceSettings {
	if platform.IsAndroidSerial(udid) {
		return r.Android
	}
	return r.IOS
}

// SetAppearance implements DeviceSettings with platform routing.
func (r SettingsRouter) SetAppearance(ctx context.Context, udid string, dark bool) error {
	return r.pick(udid).SetAppearance(ctx, udid, dark)
}

// SetLocation implements DeviceSettings with platform routing.
func (r SettingsRouter) SetLocation(ctx context.Context, udid string, lat, lon float64) error {
	return r.pick(udid).SetLocation(ctx, udid, lat, lon)
}

// SetPermission implements DeviceSettings with platform routing.
func (r SettingsRouter) SetPermission(ctx context.Context, udid, appID, permission string, grant bool) error {
	return r.pick(udid).SetPermission(ctx, udid, appID, permission, grant)
}

// SetSettings enables POST /api/devices/{udid}/settings. Without it the
// route answers 501.
func (s *Server) SetSettings(d DeviceSettings) { s.settings = d }

// settingsRequest is one settings call: any subset of appearance, location,
// and permissions to grant or revoke for app. Everything given is applied,
// in that order.
type settingsRequest struct {
	Appearance string    `json:"appearance"`
	Location   *geoPoint `json:"location"`
	App        string    `json:"app"`
	Grant      []string  `json:"grant"`
	Revoke     []string  `json:"revoke"`
}

// geoPoint is a latitude and longitude in degrees.
type geoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// validate refuses a request that could not be applied as a whole, before
// any of it is, so a bad field never leaves the device half-changed.
func (q settingsRequest) validate() error {
	switch {
	case q.Appearance != "" && q.Appearance != "dark" && q.Appearance != "light":
		return errors.New(`appearance must be "dark" or "light"`)
	case q.Location != nil && (q.Location.Lat < -90 || q.Location.Lat > 90 || q.Location.Lon < -180 || q.Location.Lon > 180):
		return errors.New("location needs lat in [-90, 90] and lon in [-180, 180]")
	case len(q.Grant)+len(q.Revoke) > 0 && q.App == "":
		return errors.New("grant and revoke need app, the bundle id or package the permission is for")
	case q.Appearance == "" && q.Location == nil && len(q.Grant)+len(q.Revoke) == 0:
		return errors.New("nothing to set: give appearance, location, or grant/revoke with app")
	}
	return nil
}

// handleSettings applies a settings request and reports what it applied. A
// failure part-way says what was already applied, since that is not undone.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		httpError(w, http.StatusNotImplemented, errors.New("device settings are not available on this server"))
		return
	}
	var req settingsRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := req.validate(); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := deviceOp(r)
	defer cancel()
	applied, err := s.applySettings(ctx, r.PathValue("udid"), req)
	if err != nil {
		if len(applied) > 0 {
			err = fmt.Errorf("%w (already applied: %s)", err, strings.Join(applied, "; "))
		}
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "action": "settings", "applied": applied})
}

// applySettings applies each part of req in turn, stopping at the first
// failure, and returns what it applied.
func (s *Server) applySettings(ctx context.Context, udid string, req settingsRequest) ([]string, error) {
	var steps []func() (string, error)
	if req.Appearance != "" {
		steps = append(steps, func() (string, error) {
			return "appearance " + req.Appearance, s.settings.SetAppearance(ctx, udid, req.Appearance == "dark")
		})
	}
	if p := req.Location; p != nil {
		steps = append(steps, func() (string, error) {
			return fmt.Sprintf("location %g,%g", p.Lat, p.Lon), s.setLocation(ctx, udid, *p)
		})
	}
	steps = append(steps, s.permissionSteps(ctx, udid, req.App, req.Grant, true)...)
	steps = append(steps, s.permissionSteps(ctx, udid, req.App, req.Revoke, false)...)
	var applied []string
	for _, step := range steps {
		done, err := step()
		if err != nil {
			return applied, err
		}
		applied = append(applied, done)
	}
	return applied, nil
}

// setLocation sets the device's location and, when it is recording, adds
// the step to the flow: Maestro's setLocation replays it on any device.
// Appearance and permissions have no Maestro step, so they are not recorded.
func (s *Server) setLocation(ctx context.Context, udid string, p geoPoint) error {
	if err := s.settings.SetLocation(ctx, udid, p.Lat, p.Lon); err != nil {
		return err
	}
	s.capture.OnLocation(udid, p.Lat, p.Lon)
	return nil
}

// permissionSteps is one step per permission to grant (or revoke).
func (s *Server) permissionSteps(ctx context.Context, udid, app string, names []string, grant bool) []func() (string, error) {
	verb := "revoke "
	if grant {
		verb = "grant "
	}
	steps := make([]func() (string, error), 0, len(names))
	for _, name := range names {
		steps = append(steps, func() (string, error) {
			return verb + name, s.settings.SetPermission(ctx, udid, app, name, grant)
		})
	}
	return steps
}
