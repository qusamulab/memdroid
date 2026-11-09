package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MiB request-body cap

// secure caps the request body size and, when a token is configured, enforces
// it on /api and /ws paths. Presenting ?token=<token> sets a cookie so the
// browser UI keeps working after the first navigation.
func secure(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Only persist a credential that actually checks out, so a bad
		// ?token= cannot plant a cookie that fails every later request.
		if q := r.URL.Query().Get("token"); secretEqual(q, token) {
			http.SetCookie(w, &http.Cookie{
				Name:     "mdtoken",
				Value:    q,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		protected := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/")
		if protected && !tokenOK(r, token) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenOK compares the presented credential in constant time so a remote caller
// cannot recover the token byte-by-byte from response latency.
func tokenOK(r *http.Request, token string) bool {
	if secretEqual(r.URL.Query().Get("token"), token) {
		return true
	}
	if c, err := r.Cookie("mdtoken"); err == nil && secretEqual(c.Value, token) {
		return true
	}
	// Require the scheme rather than accepting a bare header value, so the
	// contract matches what the docs promise.
	if h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && secretEqual(h, token) {
		return true
	}
	return false
}

func secretEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// originChecked rejects cross-origin WebSocket handshakes.
//
// x/net/websocket does not validate Origin, so without this any web page the
// user visits could open a socket to a loopback memdroid and read the live
// watch feed.
func originChecked(listenAddr string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !sameOrigin(origin, listenAddr, r.Host) {
			writeError(w, http.StatusForbidden, "cross-origin WebSocket connections are not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether origin points at this server.
func sameOrigin(origin, listenAddr, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == host {
		return true
	}
	// A loopback listener is reachable under several equivalent names.
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return false
	}
	for _, h := range []string{"localhost:" + port, "127.0.0.1:" + port, "[::1]:" + port} {
		if u.Host == h {
			return true
		}
	}
	return false
}
