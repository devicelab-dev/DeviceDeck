package server

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestInputOwnersRefusesSecondClaim(t *testing.T) {
	o := newInputOwners()
	release, err := o.claim("AAA", "client-1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	_, err = o.claim("AAA", "client-2")
	if err == nil {
		t.Fatal("a second driver must be refused, not silently share the device")
	}
	// The refusal has to name the holder — a bare "busy" leaves the user
	// with the same mystery the refusal exists to prevent.
	if !strings.Contains(err.Error(), "client-1") {
		t.Errorf("refusal does not identify the holder: %v", err)
	}
	if !strings.Contains(err.Error(), "single worker") {
		t.Errorf("refusal does not say how to fix it: %v", err)
	}
	// A different device is unaffected.
	if _, err := o.claim("BBB", "client-2"); err != nil {
		t.Errorf("claiming another device: %v", err)
	}
	release()
	if _, err := o.claim("AAA", "client-2"); err != nil {
		t.Errorf("device should be free after release: %v", err)
	}
}

func TestInputOwnersHeldBy(t *testing.T) {
	o := newInputOwners()
	if got := o.heldBy("AAA"); got != "" {
		t.Errorf("free device reports %q", got)
	}
	release, _ := o.claim("AAA", "client-1")
	if got := o.heldBy("AAA"); got != "client-1" {
		t.Errorf("heldBy = %q", got)
	}
	release()
	if got := o.heldBy("AAA"); got != "" {
		t.Errorf("released device reports %q", got)
	}
}

// A release arriving after the device has been claimed again must not
// evict the new holder — otherwise a slow disconnect silently hands the
// device to a third client mid-session.
func TestInputOwnersReleaseDoesNotEvictSuccessor(t *testing.T) {
	o := newInputOwners()
	staleRelease, _ := o.claim("AAA", "client-1")
	o.releaseClaim("AAA", inputClaim{client: "client-1", since: time.Time{}}) // not the held claim
	if got := o.heldBy("AAA"); got != "client-1" {
		t.Fatalf("a non-matching release evicted the holder: %q", got)
	}
	staleRelease()

	second, err := o.claim("AAA", "client-2")
	if err != nil {
		t.Fatalf("second claim after release: %v", err)
	}
	staleRelease() // the first client's release, arriving late
	if got := o.heldBy("AAA"); got != "client-2" {
		t.Errorf("late release evicted the successor: %q", got)
	}
	second()
}

func TestInputOwnersRefusalReportsDuration(t *testing.T) {
	o := newInputOwners()
	base := time.Now()
	o.now = func() time.Time { return base }
	if _, err := o.claim("AAA", "client-1"); err != nil {
		t.Fatal(err)
	}
	o.now = func() time.Time { return base.Add(90 * time.Second) }
	_, err := o.claim("AAA", "client-2")
	if err == nil || !strings.Contains(err.Error(), "1m30s") {
		t.Errorf("refusal should say how long it has been held: %v", err)
	}
}

func TestTruncateReason(t *testing.T) {
	short := "device busy"
	if got := truncateReason(short); got != short {
		t.Errorf("short reason altered: %q", got)
	}
	long := strings.Repeat("x", 400)
	got := truncateReason(long)
	if len(got) > wsCloseReasonMax {
		t.Errorf("reason not truncated: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncation should be visible: %q", got[len(got)-4:])
	}
}

// A reason ending in a multi-byte rune must not be cut through it. The
// single ASCII prefix is deliberate: it pushes the cut off a rune
// boundary, which is the case the back-up loop exists for and which an
// aligned string never exercises.
func TestTruncateReasonKeepsRunesIntact(t *testing.T) {
	got := truncateReason("x" + strings.Repeat("…", 200))
	if len(got) > wsCloseReasonMax {
		t.Fatalf("not truncated: %d bytes", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation split a rune: %q", got)
	}
	// Aligned input still truncates correctly.
	aligned := truncateReason(strings.Repeat("é", 200))
	if len(aligned) > wsCloseReasonMax || !utf8.ValidString(aligned) {
		t.Errorf("aligned truncation wrong: %d bytes, valid=%v", len(aligned), utf8.ValidString(aligned))
	}
}

func TestInputOwnersTakeOver(t *testing.T) {
	o := newInputOwners()
	var kickedBy string
	first, err := o.acquire("AAA", "tab-1", func(by string) { kickedBy = by }, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := o.acquire("AAA", "tab-2", nil, true)
	if err != nil {
		t.Fatalf("take over refused: %v", err)
	}
	if kickedBy != TakenOverPrefix+"tab-2" || o.heldBy("AAA") != "tab-2" {
		t.Errorf("kickedBy=%q holder=%q, want tab-2 for both", kickedBy, o.heldBy("AAA"))
	}
	first() // the kicked holder's release must not free its successor's device
	if o.heldBy("AAA") != "tab-2" {
		t.Error("the kicked holder's release evicted the one that took over")
	}
	// A holder that cannot be kicked is still replaced.
	if _, err := o.acquire("AAA", "tab-3", nil, true); err != nil || o.heldBy("AAA") != "tab-3" {
		t.Errorf("take over from an unkickable holder: err=%v holder=%q", err, o.heldBy("AAA"))
	}
	second()
}
