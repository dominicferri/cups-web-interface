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
	unsubscribe()
	srv.touch()
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

func TestWaitIdleCountsAPIRequestsOnly(t *testing.T) {
	srv, _ := setupTestServer()
	handler := srv.Handler()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/favicon.svg", nil))
	if srv.lastActivity.Load() != 0 {
		t.Fatal("a static file request counted as activity")
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6310/api/v1/auth/status", nil))
	if srv.lastActivity.Load() == 0 {
		t.Fatal("an /api/ request did not count as activity")
	}
}
