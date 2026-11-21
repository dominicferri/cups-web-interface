package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/PaperCutSoftware/cups-web-interface/internal/api"
	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
)

func hostPort(t *testing.T, ts *httptest.Server) (string, int) {
	t.Helper()
	addr := ts.Listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestServerAnswersLiveServerAndResetsIdleTimer(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("connection owner lookup is only implemented on macOS")
	}
	srv := api.NewServer(cups.NewMockAdapter(), nil, "test-token")
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Config.ConnContext = srv.ConnContext
	ts.Start()
	defer ts.Close()

	host, port := hostPort(t, ts)
	ok, err := serverAnswers(host, port)
	if err != nil || !ok {
		t.Fatalf("serverAnswers = %v, %v; want true, nil", ok, err)
	}

	// With no start window, WaitIdle returns at once unless the request counted.
	done := make(chan struct{})
	go func() {
		srv.WaitIdle(10*time.Second, 0, 5*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("the request did not reset the idle timer")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServerAnswersRejectsOtherServers(t *testing.T) {
	notWebUI := httptest.NewServer(http.NotFoundHandler())
	host, port := hostPort(t, notWebUI)
	if ok, err := serverAnswers(host, port); ok || err != nil {
		t.Fatalf("a server without the API: serverAnswers = %v, %v; want false, nil", ok, err)
	}

	notWebUI.Close()
	if ok, err := serverAnswers(host, port); ok || err != nil {
		t.Fatalf("a closed port: serverAnswers = %v, %v; want false, nil", ok, err)
	}
}
