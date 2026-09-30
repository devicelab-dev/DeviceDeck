package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// The server listens on the network by default so teammates can share
// devices, which also lets anyone on that network view and drive them. An
// access token (devicedeck --token, or DEVICEDECK_TOKEN) closes that: every
// request must carry it. It tells people with the link from people without
// it; it is not a user system.
//
// A client proves it three ways, so each kind of client can: scripts and the
// MCP send `Authorization: Bearer <token>`; a browser opens a page once with
// `?token=<token>` and gets a cookie, which its later requests and WebSocket
// connections carry by themselves — so a Playwright test changes only the
// first URL it opens.

// TokenCookie holds the access token in a browser that opened a page with it.
const TokenCookie = "devicedeck_token"

// errNeedToken is the 401 body: how to get in, not just "no".
var errNeedToken = errors.New("this DeviceDeck needs its access token: open the link it printed " +
	"(…?token=<token>) once in this browser, or send Authorization: Bearer <token>")

// SetAccessToken makes every request need token. Empty leaves the server
// open, as it is by default.
func (s *Server) SetAccessToken(token string) { s.token = token }

// requireToken admits a request that carries token (header, cookie, or query
// — which also sets the cookie) and answers 401 to any other. With no token
// set it is a pass-through.
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case matches(bearer(r), token), matches(cookie(r), token):
		case matches(r.URL.Query().Get("token"), token):
			// Secure only over TLS: the server speaks plain HTTP, and a browser
			// never sends a Secure cookie back over it.
			http.SetCookie(w, &http.Cookie{
				Name: TokenCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
				Secure: r.TLS != nil,
			})
		default:
			httpError(w, http.StatusUnauthorized, errNeedToken)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// matches compares in constant time, so response timing does not leak how
// much of a guess was right.
func matches(got, want string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// bearer is the token in an `Authorization: Bearer …` header, or "".
func bearer(r *http.Request) string {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}

// cookie is the token a browser holds from an earlier ?token=, or "".
func cookie(r *http.Request) string {
	c, err := r.Cookie(TokenCookie)
	if err != nil {
		return ""
	}
	return c.Value
}
