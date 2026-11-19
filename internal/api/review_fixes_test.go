package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
)

func TestGetJobsRejectsUnsafeFilters(t *testing.T) {
	srv, _ := setupTestServer()
	for _, target := range []string{
		"/api/v1/jobs?printer=-hevil.example:631",
		"/api/v1/jobs?printer=a%20b",
		"/api/v1/jobs?which=everything",
	} {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, newTestReq(srv, "GET", target, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, w.Code)
		}
	}

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, newTestReq(srv, "GET", "/api/v1/jobs?which=all&printer=Office_Laser", nil))
	if w.Code != http.StatusOK {
		t.Errorf("valid jobs query = %d, want 200", w.Code)
	}
}

func TestServerSettingsPartialBodyKeepsOtherFlags(t *testing.T) {
	srv, mock := setupTestServer()
	if err := mock.SetServerSettings(cups.ServerSettings{SharePrinters: true, RemotePrinters: true, DebugLogging: false}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, newTestReq(srv, "PUT", "/api/v1/server/settings", strings.NewReader(`{"debug_logging":true}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT partial settings = %d, want 200", w.Code)
	}

	got, _ := mock.GetServerSettings()
	if want := (cups.ServerSettings{SharePrinters: true, RemotePrinters: true, DebugLogging: true}); *got != want {
		t.Errorf("settings after partial update = %+v, want %+v", *got, want)
	}
}

func TestDriverSwapAndAddPrinterWithUploadedPPD(t *testing.T) {
	srv, mock := setupTestServer()
	res, err := mock.ValidatePPD([]byte("*PPD-Adobe: \"4.3\"\n*ModelName: \"Acme 9000\"\n"))
	if err != nil || res.UploadID == "" {
		t.Fatalf("ValidatePPD = %+v, %v", res, err)
	}

	do := func(method, target, body string) int {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, newTestReq(srv, method, target, strings.NewReader(body)))
		return w.Code
	}

	add := `{"name":"Uploaded_Q","uri":"ipp://10.0.0.5/ipp/print","ppd_upload_id":"` + res.UploadID + `"}`
	if code := do("POST", "/api/v1/printers", add); code != http.StatusOK && code != http.StatusCreated {
		t.Errorf("add printer with upload ID = %d, want success", code)
	}
	if code := do("PUT", "/api/v1/printers/Uploaded_Q/driver", `{"ppd_upload_id":"`+res.UploadID+`"}`); code != http.StatusOK {
		t.Errorf("driver swap with upload ID = %d, want 200", code)
	}

	if code := do("PUT", "/api/v1/printers/Uploaded_Q/driver", `{"ppd_upload_id":"../../etc/passwd"}`); code != http.StatusBadRequest {
		t.Errorf("driver swap with malformed upload ID = %d, want 400", code)
	}
	if code := do("POST", "/api/v1/printers", `{"name":"Bad_Q","uri":"ipp://10.0.0.5/ipp/print","ppd_upload_id":"nope"}`); code != http.StatusBadRequest {
		t.Errorf("add printer with malformed upload ID = %d, want 400", code)
	}
	if code := do("PUT", "/api/v1/printers/Uploaded_Q/driver", `{}`); code != http.StatusBadRequest {
		t.Errorf("driver swap with neither driver nor upload = %d, want 400", code)
	}
}

func TestLoopbackHostRules(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "localhost": true, "127.0.0.2": true,
		"0.0.0.0": false, "::": false, "192.168.1.10": false, "example.com": false, "": false,
	} {
		if got := IsLoopbackHost(host); got != want {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}

	srv, _ := setupTestServer()
	req := newTestReq(srv, "GET", "/api/v1/printers", nil)
	req.Host = "0.0.0.0:6310"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("request with Host 0.0.0.0 = %d, want 400", w.Code)
	}
}

// The event stream must outlive the server's WriteTimeout.
func TestEventStreamSurvivesWriteTimeout(t *testing.T) {
	srv, _ := setupTestServer()
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Config.WriteTimeout = 200 * time.Millisecond
	ts.Start()
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events", nil)
	req.Header.Set("X-CUPS-WebUI-Token", srv.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d, want 200", resp.StatusCode)
	}

	time.Sleep(500 * time.Millisecond)
	srv.events.Broadcast(SSEEvent{Type: "job_update"})

	got := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if line, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				got <- line
				return
			}
		}
		got <- ""
	}()

	select {
	case line := <-got:
		var ev SSEEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "job_update" {
			t.Errorf("event after the write timeout = %q, want a job_update event", line)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("no event arrived after the write timeout")
	}
}
