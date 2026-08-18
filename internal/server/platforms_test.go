package server

import (
	"context"
	"errors"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

type stubLister struct {
	devices []sim.Device
	err     error
}

func (s stubLister) Booted(context.Context) ([]sim.Device, error) { return s.devices, s.err }

func TestMultiLister(t *testing.T) {
	iosDev := sim.Device{UDID: "IOS-1", Booted: true}
	androidDev := sim.Device{UDID: "emulator-5554", Booted: true}

	t.Run("concatenates sources", func(t *testing.T) {
		m := MultiLister{stubLister{devices: []sim.Device{iosDev}}, stubLister{devices: []sim.Device{androidDev}}}
		devices, err := m.Booted(context.Background())
		if err != nil || len(devices) != 2 {
			t.Fatalf("devices = %+v, err %v", devices, err)
		}
	})

	t.Run("one broken source does not hide the other", func(t *testing.T) {
		m := MultiLister{stubLister{err: errors.New("adb missing")}, stubLister{devices: []sim.Device{iosDev}}}
		devices, err := m.Booted(context.Background())
		if err != nil || len(devices) != 1 || devices[0].UDID != "IOS-1" {
			t.Fatalf("devices = %+v, err %v", devices, err)
		}
	})

	t.Run("all sources failing surfaces the error", func(t *testing.T) {
		m := MultiLister{stubLister{err: errors.New("a")}, stubLister{err: errors.New("b")}}
		if _, err := m.Booted(context.Background()); err == nil {
			t.Fatal("expected error when every source fails")
		}
	})
}

type stubShots struct{ tag string }

func (s stubShots) Screenshot(context.Context, string) ([]byte, error) { return []byte(s.tag), nil }

func TestScreenshotRouter(t *testing.T) {
	r := ScreenshotRouter{IOS: stubShots{"ios"}, Android: stubShots{"android"}}
	if png, _ := r.Screenshot(context.Background(), "emulator-5554"); string(png) != "android" {
		t.Errorf("android screenshot routed to %q", png)
	}
	if png, _ := r.Screenshot(context.Background(), "EB69B42A-0000"); string(png) != "ios" {
		t.Errorf("ios screenshot routed to %q", png)
	}
}
