package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
)

// setupUntrustedServer returns a server that treats every connection as coming
// from someone other than the user running it (another local account or host).
func setupUntrustedServer() *Server {
	srv, _ := setupTestServer()
	srv.peerIsSameUser = func(*http.Request) bool { return false }
	return srv
}

func serve(srv *Server, req *http.Request) *httptest.ResponseRecorder {
	req.Host = "127.0.0.1:6310"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestUntrustedPeerGetsNoCookieOnNavigation(t *testing.T) {
	srv := setupUntrustedServer()
	for _, path := range []string{"/", "/index.html"} {
		w := serve(srv, httptest.NewRequest("GET", path, nil))
		if c := w.Header().Get("Set-Cookie"); c != "" {
			t.Fatalf("GET %s: untrusted peer must not receive a session cookie, got %q", path, c)
		}
	}
}

func TestUntrustedPeerCannotObtainToken(t *testing.T) {
	srv := setupUntrustedServer()
	req := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	req.Header.Set("X-Requested-With", "CUPS-WebUI")
	w := serve(srv, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for untrusted peer, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), testSecretToken) {
		t.Fatal("response to untrusted peer leaked the session token")
	}
	if c := w.Header().Get("Set-Cookie"); c != "" {
		t.Fatalf("untrusted peer must not receive a session cookie, got %q", c)
	}
}

func TestUntrustedPeerWithTokenCanEstablishSession(t *testing.T) {
	srv := setupUntrustedServer()

	// Session refresh with the token already in hand.
	req := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	req.Header.Set("X-Requested-With", "CUPS-WebUI")
	req.Header.Set("X-CUPS-WebUI-Token", testSecretToken)
	if w := serve(srv, req); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Set-Cookie"), SessionCookieName) {
		t.Fatalf("expected 200 with cookie for token holder, got %d, cookie %q", w.Code, w.Header().Get("Set-Cookie"))
	}

	// The session URL printed at startup and used by `cups-webui open`.
	w := serve(srv, httptest.NewRequest("GET", "/?token="+testSecretToken, nil))
	if !strings.Contains(w.Header().Get("Set-Cookie"), SessionCookieName) {
		t.Fatalf("expected a session cookie for a valid ?token= link, got %q", w.Header().Get("Set-Cookie"))
	}

	// A wrong token gets nothing.
	w = serve(srv, httptest.NewRequest("GET", "/?token=not-the-token", nil))
	if c := w.Header().Get("Set-Cookie"); c != "" {
		t.Fatalf("wrong token must not receive a session cookie, got %q", c)
	}
}

func TestUntrustedPeerCannotReadAPI(t *testing.T) {
	srv := setupUntrustedServer()
	for _, path := range []string{
		"/api/v1/jobs",
		"/api/v1/printers",
		"/api/v1/system/status",
		"/api/v1/system/logs",
		"/api/v1/server/settings",
		"/api/v1/events",
	} {
		if w := serve(srv, httptest.NewRequest("GET", path, nil)); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: expected 401 for untrusted peer without token, got %d", path, w.Code)
		}
	}

	// With the token, reads work.
	req := httptest.NewRequest("GET", "/api/v1/jobs", nil)
	req.Header.Set("X-CUPS-WebUI-Token", testSecretToken)
	if w := serve(srv, req); w.Code != http.StatusOK {
		t.Fatalf("expected 200 for untrusted peer with token, got %d", w.Code)
	}

	// Mutations without the token stay rejected.
	if w := serve(srv, httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for untrusted mutation, got %d", w.Code)
	}
}

func TestUntrustedPeerAuthStatusReportsUnauthenticated(t *testing.T) {
	srv := setupUntrustedServer()
	w := serve(srv, httptest.NewRequest("GET", "/api/v1/auth/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from auth status, got %d", w.Code)
	}
	var res map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %+v", res)
	}
}

func TestTrustedPeerStillNeedsTokenToMutate(t *testing.T) {
	srv, _ := setupTestServer() // trusted peer
	if w := serve(srv, httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for mutation without token even from a trusted peer, got %d", w.Code)
	}
	if w := serve(srv, httptest.NewRequest("GET", "/api/v1/jobs", nil)); w.Code != http.StatusOK {
		t.Fatalf("expected 200 for read from a trusted peer, got %d", w.Code)
	}
}

// Without connection state the default check must fail closed.
func TestDefaultPeerCheckFailsClosed(t *testing.T) {
	srv := NewServer(cups.NewMockAdapter(), nil, testSecretToken)

	req := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	req.Header.Set("X-Requested-With", "CUPS-WebUI")
	if w := serve(srv, req); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when the connection owner is unknown, got %d", w.Code)
	}

	srv.peerIsSameUser = nil
	if w := serve(srv, httptest.NewRequest("GET", "/api/v1/jobs", nil)); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no peer check configured, got %d", w.Code)
	}
}

// End to end over a real loopback connection: a client run by the same user is
// signed in automatically, exactly as when opening the address in a browser.
func TestRealLoopbackConnectionFromSameUserIsTrusted(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("connection owner lookup is only implemented on macOS")
	}
	srv := NewServer(cups.NewMockAdapter(), nil, testSecretToken)
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Config.ConnContext = srv.ConnContext
	ts.Start()
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !strings.Contains(res.Header.Get("Set-Cookie"), SessionCookieName) {
		t.Fatalf("expected a session cookie on direct navigation, got %q", res.Header.Get("Set-Cookie"))
	}

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/session", nil)
	req.Header.Set("X-Requested-With", "CUPS-WebUI")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	if res.StatusCode != http.StatusOK || body["token"] != testSecretToken {
		t.Fatalf("expected 200 with token for same-user connection, got %d %+v", res.StatusCode, body)
	}

	res, err = http.Get(ts.URL + "/api/v1/jobs")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for read over same-user connection, got %d", res.StatusCode)
	}
}
