package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fillFake is an engine that can fill, recording what it was asked.
type fillFake struct {
	fakeEngine
	got FillRequest
	err error
}

func (f *fillFake) Fill(_ context.Context, req FillRequest) error { f.got = req; return f.err }

func TestEnginesFillRoutes(t *testing.T) {
	filler := &fillFake{err: errors.New("mismatch")}
	s := &Engines{engines: map[string]engineAPI{"fills": filler, "plain": &fakeEngine{}},
		start: func(context.Context, string) (engineAPI, error) { return nil, errors.New("no device") }}
	req := FillRequest{Identifier: "u", Text: "x"}
	if err := s.Fill(context.Background(), "fills", req); err == nil || filler.got != req {
		t.Errorf("fill = %v, got %+v", err, filler.got)
	}
	if err := s.Fill(context.Background(), "plain", req); !errors.Is(err, errFillUnsupported) {
		t.Errorf("plain engine: %v", err)
	}
	if err := s.Fill(context.Background(), "gone", req); err == nil {
		t.Error("a start failure must surface")
	}
}

func TestAndroidFill(t *testing.T) {
	tests := []struct {
		name string
		drv  *fakeDriver
		req  FillRequest
		want string // error substring, "" for success
		held string // what the field must hold after
	}{
		{"by identifier", &fakeDriver{field: "old"}, FillRequest{Identifier: "username-input", Text: "devicelab"}, "", "devicelab"},
		{"by focus", &fakeDriver{field: "old"}, FillRequest{X: 50, Y: 60, Text: "devicelab"}, "", "devicelab"},
		{"to empty", &fakeDriver{field: "old"}, FillRequest{Identifier: "u"}, "", ""},
		{"masked password", &fakeDriver{mask: true}, FillRequest{Identifier: "p", Text: "robustest"}, "", "robustest"},
		{"keys dropped", &fakeDriver{ignoreKeys: true}, FillRequest{Identifier: "u", Text: "devicelab"}, "device holds", ""},
		{"not found", &fakeDriver{failing: map[string]bool{"UI.findElement": true}}, FillRequest{Identifier: "u", Text: "a"}, "find field", ""},
		{"no focus", &fakeDriver{failing: map[string]bool{"UI.activeElement": true}}, FillRequest{Text: "a"}, "focused field", ""},
		{"tap fails", &fakeDriver{failing: map[string]bool{"Gesture.click": true}}, FillRequest{Text: "a"}, "focus field", ""},
		{"clear fails", &fakeDriver{failing: map[string]bool{"Input.clearElement": true}}, FillRequest{Identifier: "u", Text: "a"}, "clear field", ""},
		{"keys fail", &fakeDriver{failing: map[string]bool{"Input.sendKeys": true}}, FillRequest{Identifier: "u", Text: "a"}, "fill field", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newTestAndroidEngine(t, tt.drv)
			err := e.Fill(context.Background(), tt.req)
			if tt.want == "" && err != nil {
				t.Fatalf("Fill: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if tt.want == "" && tt.drv.field != tt.held {
				t.Errorf("field holds %q, want %q", tt.drv.field, tt.held)
			}
		})
	}
}

// The read-back after typing fails on its own: the text went in, but the
// field can no longer be found to confirm it.
func TestAndroidFillReadBackFails(t *testing.T) {
	e, _ := newTestAndroidEngine(t, &fakeDriver{failSecondFind: true})
	err := e.Fill(context.Background(), FillRequest{Identifier: "u", Text: "a"})
	if err == nil || !strings.Contains(err.Error(), "find field") {
		t.Errorf("err = %v", err)
	}
}

func TestIsMaskedAndRegexpQuote(t *testing.T) {
	masked := []struct {
		got, want string
		ok        bool
	}{
		{"•••", "abc", true},
		{"***", "abc", true},
		{"••", "abc", false},
		{"", "", false},
		{"abc", "abc", false},
	}
	for _, m := range masked {
		if isMasked(m.got, m.want) != m.ok {
			t.Errorf("isMasked(%q, %q) = %v", m.got, m.want, !m.ok)
		}
	}
	if got := regexpQuote(`a.b(c)`); got != `a\.b\(c\)` {
		t.Errorf("regexpQuote = %q", got)
	}
}
