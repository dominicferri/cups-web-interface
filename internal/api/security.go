package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// SessionCookieName is the cookie name used to store the authenticated session secret.
const SessionCookieName = "cups_webui_session"

// GenerateSessionToken creates a cryptographically secure 16-byte random hex token.
func GenerateSessionToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Token returns the active session token for the server.
func (s *Server) Token() string {
	return s.token
}

// SetToken updates the server's expected session token.
// Empty values are ignored so the server can never be left without a secret.
func (s *Server) SetToken(token string) {
	if token == "" {
		return
	}
	s.token = token
}

// isAllowedHost verifies that the Host header matches a loopback or local binding address.
func (s *Server) isAllowedHost(hostHeader string) bool {
	h := hostHeader
	if host, _, err := net.SplitHostPort(hostHeader); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	switch strings.ToLower(h) {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0":
		return true
	default:
		return false
	}
}

// isAllowedOrigin checks that origin (an Origin or Referer header value) is the
// address this request was served on: same host and same port. Other loopback
// ports are different origins and are not trusted, because any local web
// server (or another user's) could otherwise ask for the session token.
// The comparison uses the request's Host header, so any -port value works.
func (s *Server) isAllowedOrigin(r *http.Request, origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	// The Vite dev server (npm run dev) proxies API calls from port 3000.
	if os.Getenv("CUPS_WEBUI_DEV") != "" {
		switch strings.ToLower(u.Host) {
		case "localhost:3000", "127.0.0.1:3000":
			return true
		}
	}
	return false
}

// isAuthenticated checks whether an incoming request has provided a valid session token.
// It fails closed: if no server token is configured, no request is authenticated.
func (s *Server) isAuthenticated(r *http.Request) bool {
	if s.token == "" {
		return false
	}

	// 1. Check custom header X-CUPS-WebUI-Token
	if token := r.Header.Get("X-CUPS-WebUI-Token"); token != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1 {
			return true
		}
	}

	// 2. Check Authorization Bearer header
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1 {
			return true
		}
	}

	// 3. Check session cookie
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.token)) == 1 {
			return true
		}
	}

	// 4. Check query parameter for events or initial link
	if qToken := r.URL.Query().Get("token"); qToken != "" {
		if subtle.ConstantTimeCompare([]byte(qToken), []byte(s.token)) == 1 {
			return true
		}
	}

	return false
}

// securityMiddleware applies anti-rebinding, strict CORS, and authentication controls.
func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. DNS Rebinding Defense: Validate Host header
		if r.Host != "" && !s.isAllowedHost(r.Host) {
			http.Error(w, "invalid Host header", http.StatusBadRequest)
			return
		}

		origin := r.Header.Get("Origin")

		// 2. CORS Defense: Only allow loopback origins
		if origin != "" {
			if s.isAllowedOrigin(r, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-CUPS-WebUI-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				// Third-party remote origin detected. Block preflight and mutating requests.
				if r.Method == http.MethodOptions {
					http.Error(w, "cross-origin forbidden", http.StatusForbidden)
					return
				}
				if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
					writeError(w, http.StatusForbidden, "cross-origin mutating request blocked")
					return
				}
			}
		}

		// Security response headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")

		// Handle preflight OPTIONS
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Automatically set session cookie when:
		// 1. Valid ?token= query parameter is present, OR
		// 2. Direct top-level navigation to the web interface root from allowed loopback host (not cross-site),
		//    on a connection owned by the user running this server
		shouldSetCookie := false
		if qToken := r.URL.Query().Get("token"); qToken != "" && s.token != "" {
			if subtle.ConstantTimeCompare([]byte(qToken), []byte(s.token)) == 1 {
				shouldSetCookie = true
			}
		} else if s.token != "" && (r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "") && r.Method == http.MethodGet {
			// Direct browser navigation to root from trusted loopback host
			secFetchSite := r.Header.Get("Sec-Fetch-Site")
			referer := r.Header.Get("Referer")
			refererOk := referer == "" || s.isAllowedOrigin(r, referer)
			if secFetchSite != "cross-site" && refererOk && (origin == "" || s.isAllowedOrigin(r, origin)) && s.isTrustedPeer(r) {
				shouldSetCookie = true
			}
		}

		if shouldSetCookie {
			http.SetCookie(w, &http.Cookie{
				Name:     SessionCookieName,
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		// API access rules (the auth endpoints apply their own checks):
		//   - mutating requests always require the session token, plus CSRF defense
		//   - read requests require either the token or a connection owned by the
		//     user running this server, so other local users cannot read queue data
		// Browsers mark where a request came from. The interface only calls the
		// API from its own pages (same-origin); a request marked as coming from
		// another site, or another localhost port (same-site), is refused for
		// every method, so other pages can't make the server run CUPS commands.
		// Requests without the header (curl, scripts) are judged by the rules below.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			switch r.Header.Get("Sec-Fetch-Site") {
			case "", "same-origin", "none":
			default:
				writeError(w, http.StatusForbidden, "cross-site request blocked")
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/session" {
			if r.URL.Path != "/api/v1/auth/status" && !s.isAuthenticated(r) && !s.isTrustedPeer(r) {
				writeError(w, http.StatusUnauthorized, "unauthorized: valid session token required")
				return
			}

			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
				// Modern browser cross-site metadata check
				if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
					writeError(w, http.StatusForbidden, "cross-site request blocked")
					return
				}

				if !s.isAuthenticated(r) {
					writeError(w, http.StatusUnauthorized, "unauthorized: valid session token required")
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// handleAuthStatus reports whether the current client holds a valid authenticated session.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": s.isAuthenticated(r),
	})
}

// handleAuthSession establishes or refreshes the session cookie for same-origin loopback requests
// made by the user running this server (or by a caller that already holds the token).
func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	// Must require custom header X-Requested-With: CUPS-WebUI (browsers forbid cross-origin scripts from setting this without CORS preflight)
	if r.Header.Get("X-Requested-With") != "CUPS-WebUI" {
		writeError(w, http.StatusForbidden, "missing required security header")
		return
	}

	// Must not be cross-site and must be allowed loopback origin or same-origin
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, http.StatusForbidden, "cross-site request forbidden")
		return
	}

	origin := r.Header.Get("Origin")
	if origin != "" && !s.isAllowedOrigin(r, origin) {
		writeError(w, http.StatusForbidden, "unauthorized origin")
		return
	}

	referer := r.Header.Get("Referer")
	if referer != "" && !s.isAllowedOrigin(r, referer) {
		writeError(w, http.StatusForbidden, "unauthorized referer")
		return
	}

	if s.token == "" {
		// Fail closed: never report a session as authenticated without a server secret.
		writeError(w, http.StatusInternalServerError, "server session not initialised")
		return
	}

	// Only hand out the token to a caller that already holds it, or whose
	// connection is owned by the user running this server.
	if !s.isAuthenticated(r) && !s.isTrustedPeer(r) {
		writeError(w, http.StatusForbidden, "session can only be established by the user running this service")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"authenticated": true,
		"token":         s.token,
	})
}
