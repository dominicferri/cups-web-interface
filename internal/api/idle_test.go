package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func waitIdleDone(srv *Server, grace, startWindow time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		srv.WaitIdle(grace, startWindow, 5*time.Millisecond)
		close(done)
	}()
	return done
}

func TestWaitIdleUsesStartWindowBeforeFirstRequest(t *testing.T) {
	srv, _ := setupTestServer()
	done := waitIdleDone(srv, time.Millisecond, 400*time.Millisecond)

	select {
	case <-done:
		t.Fatal("idle before the start window ended")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not idle after the start window")
	}
}

func TestWaitIdleStaysUpWhileEventStreamIsOpen(t *testing.T) {
	srv, _ := setupTestServer()
	_, unsubscribe := srv.events.Subscribe()
	done := waitIdleDone(srv, 400*time.Millisecond, 30*time.Millisecond)

	select {
	case <-done:
		t.Fatal("idle while an event stream was open")
	case <-time.After(120 * time.Millisecond):
	}

	// What handleEvents does when the browser closes the stream.
	srv.touch()
	unsubscribe()
	select {
	case <-done:
		t.Fatal("idle with no grace period after the stream closed")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("not idle after the stream closed")
	}
}

// 'cups-webui open' sends a request right after the start. That must not swap
// the start window for the shorter grace period while the browser still opens.
func TestWaitIdleKeepsStartWindowAfterEarlyRequest(t *testing.T) {
	srv, _ := setupTestServer()
	srv.touch()
	done := waitIdleDone(srv, time.Millisecond, 400*time.Millisecond)

	select {
	case <-done:
		t.Fatal("idle before the start window ended")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not idle after the start window")
	}
}

func TestWaitIdleCountsAPIRequestsOnly(t *testing.T) {
	srv, _ := setupTestServer()
	handler := srv.Handler()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/favicon.svg", nil))
	if srv.lastActivity.Load() != 0 {
		t.Fatal("a static file request counted as activity")
	}

	crossSite := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/api/v1/auth/status", nil)
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	handler.ServeHTTP(httptest.NewRecorder(), crossSite)
	if srv.lastActivity.Load() != 0 {
		t.Fatal("a rejected cross-site request counted as activity")
	}

	// Another local user without the token, on the one path that needs no sign-in.
	srv.peerIsSameUser = func(*http.Request) bool { return false }
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/api/v1/auth/status", nil))
	if srv.lastActivity.Load() != 0 {
		t.Fatal("a request from another user without the token counted as activity")
	}
	srv.peerIsSameUser = func(*http.Request) bool { return true }

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/api/v1/auth/status", nil))
	if srv.lastActivity.Load() == 0 {
		t.Fatal("an /api/ request did not count as activity")
	}
}
