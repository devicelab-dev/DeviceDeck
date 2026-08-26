package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// screen builds a one-node tree whose hashes are determined by label,
// so a test can script "the screen changes" by changing the label.
func screen(label string) []Node {
	return []Node{{Type: "Button", Label: label, Enabled: true, Frame: Rect{Width: 10, Height: 10}}}
}

// focused is the same screen with focus on its node, so only
// InteractionHash differs from screen(label).
func focused(label string) []Node {
	n := screen(label)
	n[0].Focused = true
	return n
}

// scripted returns each tree in turn and then repeats the last one
// forever, counting calls so a test can assert how many samples a wait
// took.
func scripted(trees ...[]Node) (Snapshotter, *int) {
	calls := 0
	return func(ctx context.Context) (Snapshot, error) {
		i := calls
		calls++
		if i >= len(trees) {
			i = len(trees) - 1
		}
		return Snapshot{Nodes: trees[i]}, nil
	}, &calls
}

// fast makes the waits take microseconds; the logic is what is under test.
var fast = SettleOptions{Interval: time.Microsecond, Quiet: 3, Cap: 50 * time.Millisecond}

func TestSettle(t *testing.T) {
	a, b := screen("a"), screen("b")
	cases := []struct {
		name      string
		trees     [][]Node
		after     string
		wantLabel string
		minCalls  int
	}{
		{
			name:      "returns once the screen has changed and held still",
			trees:     [][]Node{a, a, b, b, b},
			after:     InteractionHash(a),
			wantLabel: "b",
			minCalls:  5,
		},
		{
			name:      "a change alone is not enough: it must be quiet too",
			trees:     [][]Node{b, screen("c"), screen("d"), screen("e"), screen("e"), screen("e")},
			after:     InteractionHash(a),
			wantLabel: "e",
			minCalls:  6,
		},
		{
			name:      "an unchanged screen runs to the cap and reports itself",
			trees:     [][]Node{a},
			after:     InteractionHash(a),
			wantLabel: "a",
			minCalls:  4,
		},
		{
			name:      "focus moving counts as a change even though the screen did not",
			trees:     [][]Node{focused("a"), focused("a"), focused("a")},
			after:     InteractionHash(a),
			wantLabel: "a",
			minCalls:  3,
		},
		{
			name:      "no after hash asks only for quiet",
			trees:     [][]Node{a, a, a},
			after:     "",
			wantLabel: "a",
			minCalls:  3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, calls := scripted(tc.trees...)
			got, err := Settle(context.Background(), snap, tc.after, fast)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Nodes[0].Label != tc.wantLabel {
				t.Errorf("settled on %q, want %q", got.Nodes[0].Label, tc.wantLabel)
			}
			if *calls < tc.minCalls {
				t.Errorf("took %d samples, want at least %d", *calls, tc.minCalls)
			}
		})
	}
}

func TestSettleSnapshotErrorIsReturned(t *testing.T) {
	boom := errors.New("engine down")
	snap := func(ctx context.Context) (Snapshot, error) { return Snapshot{}, boom }
	if _, err := Settle(context.Background(), snap, "", fast); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

func TestSettleStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	snap, calls := scripted(screen("a"))
	cancel()
	// Cancelled before the first sleep: one sample, then out.
	got, err := Settle(ctx, snap, InteractionHash(screen("zzz")), SettleOptions{Interval: time.Hour, Cap: 2 * time.Hour})
	if err != nil || got.Nodes == nil {
		t.Fatalf("cancel should still hand back the last tree, got %v, %v", got, err)
	}
	if *calls != 1 {
		t.Errorf("took %d samples after cancel, want 1", *calls)
	}
}

func TestSettleDefaults(t *testing.T) {
	got := SettleOptions{}.withDefaults()
	if got.Interval != settleInterval || got.Quiet != settleQuiet || got.Cap != settleCap {
		t.Errorf("defaults not applied: %+v", got)
	}
	kept := SettleOptions{Interval: 1, Quiet: 2, Cap: 3}.withDefaults()
	if kept.Interval != 1 || kept.Quiet != 2 || kept.Cap != 3 {
		t.Errorf("explicit values overridden: %+v", kept)
	}
}

var fastLaunch = LaunchOptions{Window: time.Microsecond, Appear: 20 * time.Millisecond, Interval: time.Microsecond}

// splash is what an app shows before its first real screen: things to
// look at, nothing to act on.
func splash() []Node {
	return []Node{{Type: "Image", Label: "logo", Enabled: true}, {Type: "StaticText", Label: "Welcome", Enabled: true}}
}

func TestAwaitLaunched(t *testing.T) {
	t.Run("waits past the splash for a screen with a control", func(t *testing.T) {
		snap, calls := scripted(nil, splash(), splash(), screen("login"))
		if err := AwaitLaunched(context.Background(), snap, fastLaunch); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *calls != 4 {
			t.Errorf("took %d samples, want 4", *calls)
		}
	})
	t.Run("a disabled control does not count as actionable", func(t *testing.T) {
		off := screen("login")
		off[0].Enabled = false
		snap, _ := scripted(off)
		if err := AwaitLaunched(context.Background(), snap, fastLaunch); !errors.Is(err, ErrAppNotAppeared) {
			t.Fatalf("got %v, want ErrAppNotAppeared", err)
		}
	})
	t.Run("an app stuck on its splash is reported as such", func(t *testing.T) {
		snap, _ := scripted(splash())
		if err := AwaitLaunched(context.Background(), snap, fastLaunch); !errors.Is(err, ErrAppNotAppeared) {
			t.Fatalf("got %v, want ErrAppNotAppeared", err)
		}
	})
	t.Run("a snapshot error explains the timeout", func(t *testing.T) {
		boom := errors.New("engine down")
		snap := func(ctx context.Context) (Snapshot, error) { return Snapshot{}, boom }
		if err := AwaitLaunched(context.Background(), snap, fastLaunch); !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
	t.Run("cancel while waiting to appear", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		snap, _ := scripted(nil)
		err := AwaitLaunched(ctx, snap, LaunchOptions{Interval: time.Hour, Appear: 2 * time.Hour})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
	t.Run("cancel during the window", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		snap, _ := scripted(screen("login"))
		go func() { time.Sleep(5 * time.Millisecond); cancel() }()
		err := AwaitLaunched(ctx, snap, LaunchOptions{Window: time.Hour, Appear: time.Second, Interval: time.Microsecond})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
}

func TestLaunchDefaults(t *testing.T) {
	got := LaunchOptions{}.withDefaults()
	if got.Window != launchWindow || got.Appear != launchAppear || got.Interval != settleInterval {
		t.Errorf("defaults not applied: %+v", got)
	}
}
