package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
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
// takeover by default: a test hijacked halfway through is unexplainable.
// Taking over is an explicit act instead — a person on the console pressing
// Take over, because the holder is a stale tab they cannot find — and the
// client taken from is told so as it is disconnected.
type inputOwners struct {
	mu    sync.Mutex
	owned map[string]inputClaim
	now   func() time.Time
	last  uint64 // numbers claims, so a release can tell its own from a successor
}

// inputClaim is one client's hold on a device's input. kick disconnects
// the holder, telling it why (another client took over, or the session
// ended); nil means it cannot be.
type inputClaim struct {
	client string
	since  time.Time
	id     uint64
	kick   func(why string)
	// token is handed to the holder; its HTTP actions carry it back, which
	// is how an action from the holder is told from one by another client.
	token string
}

func newInputOwners() *inputOwners {
	return &inputOwners{owned: make(map[string]inputClaim), now: time.Now}
}

// claim takes input on udid for client, or reports who already holds it.
// The returned release is nil when the claim was refused.
func (o *inputOwners) claim(udid, client string) (release func(), err error) {
	release, _, err = o.acquire(udid, client, nil, false)
	return release, err
}

// acquire is claim for a client that can be disconnected (kick) and may
// take the device from its current holder (takeOver), which is kicked. It
// returns the claim token the holder sends with its HTTP actions.
func (o *inputOwners) acquire(udid, client string, kick func(why string), takeOver bool) (release func(), token string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if held, taken := o.owned[udid]; taken {
		if !takeOver {
			return nil, "", fmt.Errorf("device %s is already being driven by %s (for %s); "+
				"a device serves one driver at a time, so run your tests with a single worker",
				udid, held.client, o.now().Sub(held.since).Round(time.Second))
		}
		if held.kick != nil {
			held.kick(TakenOverPrefix + client)
		}
	}
	o.last++
	claim := inputClaim{client: client, since: o.now(), id: o.last, kick: kick, token: newClaimToken()}
	o.owned[udid] = claim
	return func() { o.releaseClaim(udid, claim) }, claim.token, nil
}

// newClaimToken is a random token for one claim.
func newClaimToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b) // crypto/rand does not fail on supported platforms
	return hex.EncodeToString(b)
}

// foreign reports who holds udid when an action carrying token did not come
// from them: "" when the device is free or the token is the holder's.
func (o *inputOwners) foreign(udid, token string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	held, ok := o.owned[udid]
	if !ok || token == held.token {
		return ""
	}
	return held.client
}

// releaseClaim drops a claim, but only if it is still the one held: a
// release racing a later claim must not evict its successor.
func (o *inputOwners) releaseClaim(udid string, claim inputClaim) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if held, ok := o.owned[udid]; ok && held.id == claim.id {
		delete(o.owned, udid)
	}
}

// kickHolder disconnects udid's driver, if any, telling it why, and frees
// the device.
func (o *inputOwners) kickHolder(udid, why string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if held, ok := o.owned[udid]; ok {
		if held.kick != nil {
			held.kick(why)
		}
		delete(o.owned, udid)
	}
}

// heldBy reports the current driver of udid, or "" when free.
func (o *inputOwners) heldBy(udid string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.owned[udid].client
}

// ClaimHeader carries a holder's claim token on its HTTP actions; the device
// page sets it on every /act.
const ClaimHeader = "X-DeviceDeck-Claim"

// WarningHeader tells a caller that its action reached a device another
// client holds.
const WarningHeader = "X-DeviceDeck-Warning"

// watchForeign wraps an action route. An action from a client other than
// the device's holder is let through — an agent may tap through the MCP
// while a browser tool holds the page, and refusing would break that — but
// it is logged and the caller is told, because two drivers interleaving is
// otherwise a silent failure.
func (s *Server) watchForeign(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		udid := r.PathValue("udid")
		if holder := s.inputs.foreign(udid, r.Header.Get(ClaimHeader)); holder != "" {
			slog.Warn("action on a device another client holds",
				"udid", udid, "path", r.URL.Path, "holder", holder, "from", r.RemoteAddr)
			w.Header().Set(WarningHeader, "device is held by "+holder+"; actions from two clients interleave")
		}
		next(w, r)
	}
}
