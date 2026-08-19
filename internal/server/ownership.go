package server

import (
	"fmt"
	"sync"
	"time"
)

// inputOwners tracks which client currently drives each device.
//
// A device serves one driver at a time. Video fans out to any number of
// viewers, because watching changes nothing, but two clients injecting
// touches into one device interleave into nonsense — and the tools this
// exists for make that the default: Playwright, Cypress and Jest all run
// test files in parallel unless told otherwise. Pointing an existing
// suite at a device is therefore enough to produce it, and the symptom
// is maddening: every spec passes alone and the suite fails together,
// with nothing in any log to say why.
//
// So a second claim is refused rather than silently shared. Refusal over
// takeover: a human whose tab went stale can close it, whereas a test
// hijacked halfway through is unexplainable.
type inputOwners struct {
	mu    sync.Mutex
	owned map[string]inputClaim
	now   func() time.Time
}

// inputClaim is one client's hold on a device's input.
type inputClaim struct {
	client string
	since  time.Time
}

func newInputOwners() *inputOwners {
	return &inputOwners{owned: make(map[string]inputClaim), now: time.Now}
}

// claim takes input on udid for client, or reports who already holds it.
// The returned release is nil when the claim was refused.
func (o *inputOwners) claim(udid, client string) (release func(), err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if held, taken := o.owned[udid]; taken {
		return nil, fmt.Errorf("device %s is already being driven by %s (for %s); "+
			"a device serves one driver at a time, so run your tests with a single worker",
			udid, held.client, o.now().Sub(held.since).Round(time.Second))
	}
	claim := inputClaim{client: client, since: o.now()}
	o.owned[udid] = claim
	return func() { o.releaseClaim(udid, claim) }, nil
}

// releaseClaim drops a claim, but only if it is still the one held: a
// release racing a later claim must not evict its successor.
func (o *inputOwners) releaseClaim(udid string, claim inputClaim) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if held, ok := o.owned[udid]; ok && held == claim {
		delete(o.owned, udid)
	}
}

// heldBy reports the current driver of udid, or "" when free.
func (o *inputOwners) heldBy(udid string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.owned[udid].client
}
