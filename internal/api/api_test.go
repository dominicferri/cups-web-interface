package api

import (
	"context"
	"encoding/json"
	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecretToken = "test-token-1234567890"

func setupTestServer() (*Server, *cups.MockAdapter) {
	mock := cups.NewMockAdapter()
	srv := NewServer(mock, nil, testSecretToken)
	// httptest requests have no real connection; treat them as coming from the
	// user running the server unless a test says otherwise.
	srv.peerIsSameUser = func(*http.Request) bool { return true }
	return srv, mock
}

func newTestReq(srv *Server, method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Host = "127.0.0.1:6310"
	if srv != nil && srv.Token() != "" {
		req.Header.Set("X-CUPS-WebUI-Token", srv.Token())
	}
	return req
}

func TestGetSystemStatus(t *testing.T) {
	srv, _ := setupTestServer()
	req := newTestReq(srv, "GET", "/api/v1/system/status", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var status cups.SystemStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if !status.DaemonRunning {
		t.Errorf("expected daemon to be running")
	}
	if status.PrintersCount != 3 {
		t.Errorf("expected 3 printers, got %d", status.PrintersCount)
	}
}

func TestGetPrinters(t *testing.T) {
	srv, _ := setupTestServer()
	req := newTestReq(srv, "GET", "/api/v1/printers", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var printers []cups.Printer
	if err := json.Unmarshal(w.Body.Bytes(), &printers); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if len(printers) != 3 {
		t.Fatalf("expected 3 printers, got %d", len(printers))
	}
	// Verify alphabetical sort order
	for i := 1; i < len(printers); i++ {
		if strings.ToLower(printers[i-1].Name) > strings.ToLower(printers[i].Name) {
			t.Errorf("printers not in alphabetical order: %s before %s", printers[i-1].Name, printers[i].Name)
		}
	}
	var hasDefault bool
	for _, p := range printers {
		if p.IsDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		t.Errorf("expected at least one default printer")
	}
}

func TestPrinterLifecycle(t *testing.T) {
	srv, _ := setupTestServer()

	// Pause printer
	reqPause := newTestReq(srv, "POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	wPause := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPause, reqPause)
	if wPause.Code != http.StatusOK {
		t.Fatalf("expected 200 pausing printer, got %d", wPause.Code)
	}

	// Verify paused
	reqGet := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	var p cups.Printer
	_ = json.Unmarshal(wGet.Body.Bytes(), &p)
	if p.State != "stopped" {
		t.Errorf("expected state 'stopped', got '%s'", p.State)
	}

	// Resume printer
	reqResume := newTestReq(srv, "POST", "/api/v1/printers/Office_LaserJet_Pro/resume", nil)
	wResume := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wResume, reqResume)
	if wResume.Code != http.StatusOK {
		t.Fatalf("expected 200 resuming printer, got %d", wResume.Code)
	}

	// Set Default
	reqDef := newTestReq(srv, "POST", "/api/v1/printers/Warehouse_Zebra_Labels/default", nil)
	wDef := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDef, reqDef)
	if wDef.Code != http.StatusOK {
		t.Fatalf("expected 200 setting default, got %d", wDef.Code)
	}
}

func TestJobsAndCancel(t *testing.T) {
	srv, _ := setupTestServer()

	// Get active jobs
	reqJobs := newTestReq(srv, "GET", "/api/v1/jobs?which=active", nil)
	wJobs := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wJobs, reqJobs)
	if wJobs.Code != http.StatusOK {
		t.Fatalf("expected 200 getting jobs, got %d", wJobs.Code)
	}

	var jobs []cups.Job
	if err := json.Unmarshal(wJobs.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("failed to parse jobs: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("expected 3 active jobs, got %d", len(jobs))
	}

	// Cancel job 101 (stuck job)
	reqCancel := newTestReq(srv, "POST", "/api/v1/jobs/101/cancel", nil)
	wCancel := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wCancel, reqCancel)
	if wCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 cancelling job, got %d", wCancel.Code)
	}

	// Verify job is no longer active
	wJobsAfter := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wJobsAfter, reqJobs)
	var jobsAfter []cups.Job
	_ = json.Unmarshal(wJobsAfter.Body.Bytes(), &jobsAfter)
	if len(jobsAfter) != 2 {
		t.Errorf("expected 2 active jobs after cancel, got %d", len(jobsAfter))
	}
}

func TestHoldAndReleaseJob(t *testing.T) {
	srv, _ := setupTestServer()

	// Hold job 102
	reqHold := newTestReq(srv, "POST", "/api/v1/jobs/102/hold", nil)
	wHold := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wHold, reqHold)
	if wHold.Code != http.StatusOK {
		t.Fatalf("expected 200 holding job, got %d", wHold.Code)
	}

	// Verify job state is held
	jobs, _ := srv.adapter.GetJobs("active", "")
	var job102 *cups.Job
	for _, j := range jobs {
		if j.ID == "102" {
			job102 = &j
			break
		}
	}
	if job102 == nil || job102.State != "held" {
		t.Fatalf("expected job 102 to be in 'held' state")
	}

	// Release job 102
	reqRelease := newTestReq(srv, "POST", "/api/v1/jobs/102/release", nil)
	wRelease := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wRelease, reqRelease)
	if wRelease.Code != http.StatusOK {
		t.Fatalf("expected 200 releasing job, got %d", wRelease.Code)
	}

	// Verify job is pending again
	jobsAfter, _ := srv.adapter.GetJobs("active", "")
	for _, j := range jobsAfter {
		if j.ID == "102" {
			if j.State != "pending" {
				t.Errorf("expected job 102 to be 'pending', got '%s'", j.State)
			}
			break
		}
	}
}

func TestRestartJob(t *testing.T) {
	srv, _ := setupTestServer()

	// Cancel job 101 first
	_ = srv.adapter.CancelJob("101")

	// Restart job 101
	reqRestart := newTestReq(srv, "POST", "/api/v1/jobs/101/restart", nil)
	wRestart := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wRestart, reqRestart)
	if wRestart.Code != http.StatusOK {
		t.Fatalf("expected 200 restarting job, got %d", wRestart.Code)
	}

	// Verify active jobs now contains 101 as pending
	jobs, _ := srv.adapter.GetJobs("active", "")
	found := false
	for _, j := range jobs {
		if j.ID == "101" && j.State == "pending" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected job 101 to be active and pending after restart")
	}
}

func TestPurgePrinterQueue(t *testing.T) {
	srv, _ := setupTestServer()

	// Purge Warehouse_Zebra_Labels queue
	reqPurge := newTestReq(srv, "POST", "/api/v1/printers/Warehouse_Zebra_Labels/purge", nil)
	wPurge := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPurge, reqPurge)
	if wPurge.Code != http.StatusOK {
		t.Fatalf("expected 200 purging queue, got %d", wPurge.Code)
	}

	// Check printer queue count is 0
	p, _ := srv.adapter.GetPrinter("Warehouse_Zebra_Labels")
	if p.QueuedJobsCount != 0 {
		t.Errorf("expected 0 queued jobs after purge, got %d", p.QueuedJobsCount)
	}
}

func TestEventBroadcaster(t *testing.T) {
	broadcaster := NewEventBroadcaster()
	ch, unsubscribe := broadcaster.Subscribe()
	defer unsubscribe()

	testEvent := SSEEvent{
		Type:   "printer_update",
		Action: "pause",
		Target: "test_printer",
	}

	go broadcaster.Broadcast(testEvent)

	select {
	case received := <-ch:
		if received.Type != testEvent.Type || received.Target != testEvent.Target {
			t.Errorf("expected event %v, got %v", testEvent, received)
		}
	}
}

func TestGetAndSetPrinterOptions(t *testing.T) {
	srv, _ := setupTestServer()

	// GET options
	reqGet := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro/options", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET options, got %d", wGet.Code)
	}

	var opts cups.PrinterOptionsResponse
	if err := json.NewDecoder(wGet.Body).Decode(&opts); err != nil {
		t.Fatalf("failed to decode options response: %v", err)
	}
	if len(opts.General) == 0 {
		t.Fatalf("expected general options, got none")
	}
	if len(opts.Accessories) == 0 {
		t.Fatalf("expected accessories options, got none")
	}

	// PUT updated option: change PageSize to A4
	body := strings.NewReader(`{"options": {"PageSize": "A4"}}`)
	reqPut := newTestReq(srv, "PUT", "/api/v1/printers/Office_LaserJet_Pro/options", body)
	reqPut.Header.Set("Content-Type", "application/json")
	wPut := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPut, reqPut)
	if wPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT options, got %d: %s", wPut.Code, wPut.Body.String())
	}

	// Verify updated option
	reqVerify := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro/options", nil)
	wVerify := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wVerify, reqVerify)
	var optsVerify cups.PrinterOptionsResponse
	_ = json.NewDecoder(wVerify.Body).Decode(&optsVerify)
	foundA4 := false
	for _, o := range optsVerify.General {
		if o.Key == "PageSize" && o.CurrentValue == "A4" {
			foundA4 = true
			break
		}
	}
	if !foundA4 {
		t.Errorf("expected PageSize to be updated to A4")
	}
}

func TestGetPrinterAudit(t *testing.T) {
	srv, _ := setupTestServer()

	// Audit Office_LaserJet_Pro (Universal)
	req := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro/audit", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for audit, got %d", w.Code)
	}

	var audit cups.ArchAuditResult
	if err := json.NewDecoder(w.Body).Decode(&audit); err != nil {
		t.Fatalf("failed to decode audit response: %v", err)
	}
	if audit.Status != "universal" || audit.RosettaRequired != false {
		t.Errorf("expected universal status without Rosetta, got %v", audit)
	}
	if len(audit.Binaries) != 2 {
		t.Errorf("expected 2 binaries in audit, got %d", len(audit.Binaries))
	}

	// Audit Finance_Secure_Copier (Rosetta 2 Dependent)
	reqRosetta := newTestReq(srv, "GET", "/api/v1/printers/Finance_Secure_Copier/audit", nil)
	wRosetta := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wRosetta, reqRosetta)
	var auditRosetta cups.ArchAuditResult
	_ = json.NewDecoder(wRosetta.Body).Decode(&auditRosetta)
	if auditRosetta.Status != "rosetta_dependent" || auditRosetta.RosettaRequired != true {
		t.Errorf("expected rosetta_dependent status with RosettaRequired=true, got %v", auditRosetta)
	}
}

func TestGetFleetAudit(t *testing.T) {
	srv, _ := setupTestServer()

	req := newTestReq(srv, "GET", "/api/v1/audit/summary", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for fleet audit, got %d", w.Code)
	}

	var summary cups.FleetAuditSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("failed to decode fleet audit: %v", err)
	}
	if summary.TotalQueues != 3 || summary.RosettaQueues != 1 || summary.UniversalQueues != 1 {
		t.Errorf("unexpected fleet audit summary: %+v", summary)
	}
}

func TestDeletePrinter(t *testing.T) {
	srv, _ := setupTestServer()

	// Delete printer
	reqDel := newTestReq(srv, "DELETE", "/api/v1/printers/Office_LaserJet_Pro", nil)
	wDel := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting printer, got %d", wDel.Code)
	}

	// Verify printer no longer exists
	reqGet := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted printer, got %d", wGet.Code)
	}
}

func TestAddAndModifyPrinter(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Add Printer
	addBody := `{"name":"Warehouse_Zebra","description":"Warehouse Zebra ZT411","uri":"socket://192.168.1.180:9100","location":"Shipping Bay 2","shared":false,"model":"zebra-zpl.ppd"}`
	reqAdd := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(addBody))
	reqAdd.Header.Set("Content-Type", "application/json")
	wAdd := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wAdd, reqAdd)
	if wAdd.Code != http.StatusCreated {
		t.Fatalf("expected 201 adding printer, got %d: %s", wAdd.Code, wAdd.Body.String())
	}

	// Verify added
	reqGet := newTestReq(srv, "GET", "/api/v1/printers/Warehouse_Zebra", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 getting added printer, got %d", wGet.Code)
	}
	var p cups.Printer
	_ = json.Unmarshal(wGet.Body.Bytes(), &p)
	if p.Name != "Warehouse_Zebra" || p.URI != "socket://192.168.1.180:9100" {
		t.Errorf("unexpected printer fields: %+v", p)
	}

	// 2. Modify Printer
	modBody := `{"description":"Shipping Zebra","uri":"socket://192.168.1.181:9100","location":"Shipping Bay 3","shared":true}`
	reqMod := newTestReq(srv, "PUT", "/api/v1/printers/Warehouse_Zebra", strings.NewReader(modBody))
	reqMod.Header.Set("Content-Type", "application/json")
	wMod := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wMod, reqMod)
	if wMod.Code != http.StatusOK {
		t.Fatalf("expected 200 modifying printer, got %d: %s", wMod.Code, wMod.Body.String())
	}

	// Verify modified
	wGetMod := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGetMod, reqGet)
	var pMod cups.Printer
	_ = json.Unmarshal(wGetMod.Body.Bytes(), &pMod)
	if pMod.Info != "Shipping Zebra" || pMod.URI != "socket://192.168.1.181:9100" || !pMod.IsShared {
		t.Errorf("expected modified fields, got: %+v", pMod)
	}
}

func TestSetPrinterDriver(t *testing.T) {
	srv, _ := setupTestServer()

	driverBody := `{"driver":"CNCUPSIRAC5840ZK.ppd"}`
	req := newTestReq(srv, "PUT", "/api/v1/printers/Office_LaserJet_Pro/driver", strings.NewReader(driverBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 updating driver, got %d: %s", w.Code, w.Body.String())
	}

	// Check printer updated
	reqGet := newTestReq(srv, "GET", "/api/v1/printers/Office_LaserJet_Pro", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	var p cups.Printer
	_ = json.Unmarshal(wGet.Body.Bytes(), &p)
	if p.Info != "CNCUPSIRAC5840ZK.ppd" || p.ArchReadiness.Status != "rosetta_dependent" {
		t.Errorf("expected updated driver info and rosetta readiness, got: %+v", p)
	}
}

func TestPrintTestPage(t *testing.T) {
	srv, _ := setupTestServer()

	req := newTestReq(srv, "POST", "/api/v1/printers/Office_LaserJet_Pro/test-page", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 printing test page, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["status"] != "ok" || res["job_id"] == nil {
		t.Errorf("expected job_id returned, got: %+v", res)
	}
}

func TestGetDrivers(t *testing.T) {
	srv, _ := setupTestServer()

	// All drivers
	req := newTestReq(srv, "GET", "/api/v1/drivers", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 getting drivers, got %d", w.Code)
	}

	var drivers []cups.DriverInfo
	_ = json.Unmarshal(w.Body.Bytes(), &drivers)
	if len(drivers) == 0 {
		t.Fatal("expected non-empty driver catalog")
	}

	// Search filter
	reqFilter := newTestReq(srv, "GET", "/api/v1/drivers?search=Zebra", nil)
	wFilter := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wFilter, reqFilter)
	var filtered []cups.DriverInfo
	_ = json.Unmarshal(wFilter.Body.Bytes(), &filtered)
	if len(filtered) != 1 || filtered[0].Make != "Zebra" {
		t.Errorf("expected 1 zebra driver, got %d", len(filtered))
	}
}

func TestValidatePPD(t *testing.T) {
	srv, _ := setupTestServer()

	validPPD := "*PPD-Adobe: \"4.3\"\n*ModelName: \"Test Model\"\n"
	req := newTestReq(srv, "POST", "/api/v1/ppds/validate", strings.NewReader(validPPD))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 validating PPD, got %d", w.Code)
	}

	var res cups.PPDValidationResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if !res.Valid || res.Model != "Test Model" {
		t.Errorf("expected valid PPD, got: %+v", res)
	}

	// Invalid PPD
	invalidPPD := "Not a ppd"
	reqInv := newTestReq(srv, "POST", "/api/v1/ppds/validate", strings.NewReader(invalidPPD))
	wInv := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wInv, reqInv)
	var resInv cups.PPDValidationResult
	_ = json.Unmarshal(wInv.Body.Bytes(), &resInv)
	if resInv.Valid || len(resInv.Errors) == 0 {
		t.Errorf("expected invalid PPD with errors, got: %+v", resInv)
	}
}

func TestServerSettings(t *testing.T) {
	srv, _ := setupTestServer()

	// GET
	reqGet := newTestReq(srv, "GET", "/api/v1/server/settings", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 getting settings, got %d", wGet.Code)
	}

	// PUT
	setBody := `{"share_printers":true,"remote_printers":false,"debug_logging":true}`
	reqPut := newTestReq(srv, "PUT", "/api/v1/server/settings", strings.NewReader(setBody))
	reqPut.Header.Set("Content-Type", "application/json")
	wPut := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPut, reqPut)
	if wPut.Code != http.StatusOK {
		t.Fatalf("expected 200 updating settings, got %d", wPut.Code)
	}

	// Verify updated
	wGet2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet2, reqGet)
	var updated cups.ServerSettings
	_ = json.Unmarshal(wGet2.Body.Bytes(), &updated)
	if !updated.SharePrinters || !updated.DebugLogging {
		t.Errorf("expected settings to update, got: %+v", updated)
	}
}

func TestGetSystemLogs(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Get all logs
	reqAll := newTestReq(srv, "GET", "/api/v1/system/logs?lines=50&level=all", nil)
	wAll := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wAll, reqAll)
	if wAll.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wAll.Code)
	}

	var logs []cups.LogEntry
	if err := json.Unmarshal(wAll.Body.Bytes(), &logs); err != nil {
		t.Fatalf("failed to decode logs JSON: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected log entries in mock mode, got 0")
	}

	// 2. Filter by error level
	reqErr := newTestReq(srv, "GET", "/api/v1/system/logs?lines=50&level=error", nil)
	wErr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wErr, reqErr)
	if wErr.Code != http.StatusOK {
		t.Fatalf("expected 200 for error logs, got %d", wErr.Code)
	}

	var errorLogs []cups.LogEntry
	if err := json.Unmarshal(wErr.Body.Bytes(), &errorLogs); err != nil {
		t.Fatalf("failed to decode error logs JSON: %v", err)
	}
	for _, l := range errorLogs {
		if l.Level != "error" {
			t.Errorf("expected level 'error', got %q", l.Level)
		}
	}

	// Verify at least one error log has diagnostic remedies attached
	hasDiag := false
	for _, l := range errorLogs {
		if l.Diagnostic != nil && len(l.Diagnostic.Remedies) > 0 {
			hasDiag = true
			break
		}
	}
	if !hasDiag {
		t.Errorf("expected error logs to have actionable diagnostic remedies attached")
	}

	// 3. Filter by warn level
	reqWarn := newTestReq(srv, "GET", "/api/v1/system/logs?lines=50&level=warn", nil)
	wWarn := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wWarn, reqWarn)
	if wWarn.Code != http.StatusOK {
		t.Fatalf("expected 200 for warn logs, got %d", wWarn.Code)
	}

	var warnLogs []cups.LogEntry
	if err := json.Unmarshal(wWarn.Body.Bytes(), &warnLogs); err != nil {
		t.Fatalf("failed to decode warn logs JSON: %v", err)
	}
	if len(warnLogs) == 0 {
		t.Errorf("expected at least 1 warning log in mock data")
	}
	for _, l := range warnLogs {
		if l.Level != "warn" && l.Level != "warning" {
			t.Errorf("expected level 'warn', got %q", l.Level)
		}
	}

	// 4. Filter by problems level (both errors and warnings)
	reqProb := newTestReq(srv, "GET", "/api/v1/system/logs?lines=50&level=problems", nil)
	wProb := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wProb, reqProb)
	if wProb.Code != http.StatusOK {
		t.Fatalf("expected 200 for problems logs, got %d", wProb.Code)
	}

	var probLogs []cups.LogEntry
	if err := json.Unmarshal(wProb.Body.Bytes(), &probLogs); err != nil {
		t.Fatalf("failed to decode problems logs JSON: %v", err)
	}
	if len(probLogs) != len(errorLogs)+len(warnLogs) {
		t.Errorf("expected %d problems logs (errors + warnings), got %d", len(errorLogs)+len(warnLogs), len(probLogs))
	}
}

func TestLaunchAgentEndpoints(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. GET status
	reqGet := newTestReq(srv, "GET", "/api/v1/system/launchagent", nil)
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 getting launchagent status, got %d", wGet.Code)
	}

	var status cups.LaunchAgentStatus
	if err := json.Unmarshal(wGet.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal LaunchAgentStatus: %v", err)
	}
	if status.Installed {
		t.Errorf("expected launchagent initially not installed in mock mode")
	}

	// 2. POST install
	reqInstall := newTestReq(srv, "POST", "/api/v1/system/launchagent", strings.NewReader(`{"action":"install"}`))
	reqInstall.Header.Set("Content-Type", "application/json")
	wInstall := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wInstall, reqInstall)
	if wInstall.Code != http.StatusOK {
		t.Fatalf("expected 200 installing launchagent, got %d", wInstall.Code)
	}

	// 3. GET status again to verify installed
	wGet2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet2, reqGet)
	var status2 cups.LaunchAgentStatus
	_ = json.Unmarshal(wGet2.Body.Bytes(), &status2)
	if !status2.Installed {
		t.Errorf("expected launchagent installed after POST install")
	}

	// 4. POST uninstall
	reqUninstall := newTestReq(srv, "POST", "/api/v1/system/launchagent", strings.NewReader(`{"action":"uninstall"}`))
	reqUninstall.Header.Set("Content-Type", "application/json")
	wUninstall := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wUninstall, reqUninstall)
	if wUninstall.Code != http.StatusOK {
		t.Fatalf("expected 200 uninstalling launchagent, got %d", wUninstall.Code)
	}

	// 5. GET status again to verify uninstalled
	wGet3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet3, reqGet)
	var status3 cups.LaunchAgentStatus
	_ = json.Unmarshal(wGet3.Body.Bytes(), &status3)
	if status3.Installed {
		t.Errorf("expected launchagent not installed after POST uninstall")
	}
}

// =========================================================================
// SECURITY & THREAT MITIGATION TESTS
// =========================================================================

func TestSecurityDNSRebinding(t *testing.T) {
	srv, _ := setupTestServer()

	// External Host header must be rejected with 400 Bad Request
	req := httptest.NewRequest("GET", "/api/v1/printers", nil)
	req.Host = "attacker.evil.com:6310"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for rebinding Host header, got %d", w.Code)
	}

	// Valid loopback hosts must be accepted
	validHosts := []string{"127.0.0.1:6310", "localhost:6310", "[::1]:6310", "127.0.0.1"}
	for _, h := range validHosts {
		reqValid := httptest.NewRequest("GET", "/api/v1/printers", nil)
		reqValid.Host = h
		wValid := httptest.NewRecorder()
		srv.Handler().ServeHTTP(wValid, reqValid)
		if wValid.Code != http.StatusOK {
			t.Errorf("expected 200 for valid host %q, got %d", h, wValid.Code)
		}
	}
}

func TestSecurityCORSAndRemoteOrigins(t *testing.T) {
	srv, _ := setupTestServer()

	// Preflight OPTIONS from unauthorized external origin must return 403 Forbidden
	reqOptions := httptest.NewRequest("OPTIONS", "/api/v1/printers", nil)
	reqOptions.Host = "127.0.0.1:6310"
	reqOptions.Header.Set("Origin", "https://malicious-website.com")
	wOptions := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wOptions, reqOptions)

	if wOptions.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-origin preflight, got %d", wOptions.Code)
	}

	// Mutating POST from external origin must be rejected with 403 Forbidden
	reqPost := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	reqPost.Host = "127.0.0.1:6310"
	reqPost.Header.Set("Origin", "https://malicious-website.com")
	reqPost.Header.Set("X-CUPS-WebUI-Token", srv.Token())
	wPost := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for external origin mutating request, got %d", wPost.Code)
	}

	// Same-origin or local loopback origin must be allowed and reflected
	reqLocal := httptest.NewRequest("OPTIONS", "/api/v1/printers", nil)
	reqLocal.Host = "127.0.0.1:6310"
	reqLocal.Header.Set("Origin", "http://127.0.0.1:6310")
	wLocal := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wLocal, reqLocal)

	if wLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 for loopback origin, got %d", wLocal.Code)
	}
	if wLocal.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:6310" {
		t.Errorf("expected Access-Control-Allow-Origin to match loopback origin, got %q", wLocal.Header().Get("Access-Control-Allow-Origin"))
	}

	// A different port on loopback is a different origin: it must not be granted
	// CORS access or be handed the session token, even on a trusted connection.
	for _, other := range []string{"http://127.0.0.1:8080", "http://localhost:6310", "http://localhost:3000"} {
		reqOther := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
		reqOther.Host = "127.0.0.1:6310"
		reqOther.Header.Set("Origin", other)
		reqOther.Header.Set("X-Requested-With", "CUPS-WebUI")
		wOther := httptest.NewRecorder()
		srv.Handler().ServeHTTP(wOther, reqOther)

		if wOther.Code != http.StatusForbidden {
			t.Errorf("origin %s: expected 403 Forbidden, got %d", other, wOther.Code)
		}
		if got := wOther.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %s: expected no Access-Control-Allow-Origin, got %q", other, got)
		}
		if strings.Contains(wOther.Body.String(), srv.Token()) {
			t.Errorf("origin %s: session token leaked in response", other)
		}
	}

	// The same check follows a custom port.
	reqPort := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	reqPort.Host = "127.0.0.1:8631"
	reqPort.Header.Set("Origin", "http://127.0.0.1:8631")
	reqPort.Header.Set("X-Requested-With", "CUPS-WebUI")
	reqPort.Header.Set("X-CUPS-WebUI-Token", srv.Token())
	wPort := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wPort, reqPort)
	if wPort.Code != http.StatusOK {
		t.Errorf("expected 200 for same-origin request on a custom port, got %d", wPort.Code)
	}
}

func TestSecurityAuthentication(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Mutating POST without token -> 401 Unauthorized
	reqNoToken := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	reqNoToken.Host = "127.0.0.1:6310"
	wNoToken := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wNoToken, reqNoToken)

	if wNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for mutating request without token, got %d", wNoToken.Code)
	}

	// 2. Mutating POST with invalid token -> 401 Unauthorized
	reqWrongToken := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	reqWrongToken.Host = "127.0.0.1:6310"
	reqWrongToken.Header.Set("X-CUPS-WebUI-Token", "wrong-token-abc")
	wWrongToken := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wWrongToken, reqWrongToken)

	if wWrongToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong token, got %d", wWrongToken.Code)
	}

	// 3. Mutating POST with Bearer token -> 200 OK
	reqBearer := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	reqBearer.Host = "127.0.0.1:6310"
	reqBearer.Header.Set("Authorization", "Bearer "+srv.Token())
	wBearer := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBearer, reqBearer)

	if wBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid Bearer token, got %d", wBearer.Code)
	}

	// 4. Mutating POST with session cookie -> 200 OK
	reqCookie := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/resume", nil)
	reqCookie.Host = "127.0.0.1:6310"
	reqCookie.AddCookie(&http.Cookie{Name: SessionCookieName, Value: srv.Token()})
	wCookie := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wCookie, reqCookie)

	if wCookie.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid session cookie, got %d", wCookie.Code)
	}

	// 5. Initial navigation with ?token= sets the session cookie
	reqNav := httptest.NewRequest("GET", "/?token="+srv.Token(), nil)
	reqNav.Host = "127.0.0.1:6310"
	wNav := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wNav, reqNav)

	cookieHeader := wNav.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, SessionCookieName) || !strings.Contains(cookieHeader, "HttpOnly") || !strings.Contains(cookieHeader, "SameSite=Strict") {
		t.Errorf("expected Set-Cookie with HttpOnly and SameSite=Strict, got: %s", cookieHeader)
	}

	// 6. Read-only GET request does not require token
	reqGet := httptest.NewRequest("GET", "/api/v1/printers", nil)
	reqGet.Host = "127.0.0.1:6310"
	wGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for read-only GET, got %d", wGet.Code)
	}
}

func TestSecuritySecFetchSiteDefense(t *testing.T) {
	srv, _ := setupTestServer()

	// Cross-site mutating request with Sec-Fetch-Site: cross-site must be rejected
	req := newTestReq(srv, "POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Sec-Fetch-Site: cross-site, got %d", w.Code)
	}
}

func TestSecurityQueueOverwriteAndValidation(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Silent Queue Overwrite Defense: attempting to add printer that already exists returns 409 Conflict
	dupBody := `{"name":"Office_LaserJet_Pro","description":"Duplicate","uri":"socket://192.168.1.50:9100"}`
	reqDup := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(dupBody))
	reqDup.Header.Set("Content-Type", "application/json")
	wDup := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDup, reqDup)

	if wDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when attempting to create duplicate queue, got %d: %s", wDup.Code, wDup.Body.String())
	}

	// 2. Invalid printer name (CLI flag injection)
	badNameBody := `{"name":"-p","description":"Flag Injection","uri":"socket://192.168.1.50:9100"}`
	reqBadName := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(badNameBody))
	reqBadName.Header.Set("Content-Type", "application/json")
	wBadName := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBadName, reqBadName)

	if wBadName.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for leading dash printer name, got %d", wBadName.Code)
	}

	// 3. Invalid printer name (Path traversal / directory separator)
	badSlashBody := `{"name":"../../../evil","description":"Traversal","uri":"socket://192.168.1.50:9100"}`
	reqBadSlash := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(badSlashBody))
	reqBadSlash.Header.Set("Content-Type", "application/json")
	wBadSlash := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBadSlash, reqBadSlash)

	if wBadSlash.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for path traversal printer name, got %d", wBadSlash.Code)
	}

	// 4. Invalid device URI scheme (file:// arbitrary read / smb injection)
	badURIBody := `{"name":"Test_Queue_Security","description":"Exploit","uri":"file:///etc/shadow"}`
	reqBadURI := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(badURIBody))
	reqBadURI.Header.Set("Content-Type", "application/json")
	wBadURI := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBadURI, reqBadURI)

	if wBadURI.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for file:// URI scheme, got %d", wBadURI.Code)
	}

	// 4b. The null device is the one accepted file: URI; look-alikes are rejected
	for uri, wantOK := range map[string]bool{
		"file:/dev/null":                  true,
		"file:///dev/null":                true,
		"file:/dev/null/../../etc/shadow": false,
		"file:/dev/zero":                  false,
	} {
		body := `{"name":"Null_Queue_Test","description":"Null","uri":"` + uri + `"}`
		req := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if gotOK := w.Code >= 200 && w.Code < 300; gotOK != wantOK {
			t.Fatalf("uri %q: got status %d, want success=%v", uri, w.Code, wantOK)
		}
		if wantOK {
			del := newTestReq(srv, "DELETE", "/api/v1/printers/Null_Queue_Test", nil)
			srv.Handler().ServeHTTP(httptest.NewRecorder(), del)
		}
	}

	// 4c. A PaperCut-tracked queue keeps its "papercut:" prefix when saved, but
	// the prefix cannot carry a blocked address past validation
	for uri, wantOK := range map[string]bool{
		"papercut:socket://192.168.1.50:9100":     true,
		"papercut:file:/dev/null":                 true,
		"papercut:file:///etc/shadow":             false,
		"papercut:exec:///bin/sh":                 false,
		"papercut:papercut:socket://192.168.1.50": false,
		"papercut:-v socket://192.168.1.50":       false,
	} {
		body := `{"uri":"` + uri + `","description":"Edited","location":"Level 2","shared":false}`
		req := newTestReq(srv, "PUT", "/api/v1/printers/Office_LaserJet_Pro", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if wantOK && w.Code != http.StatusOK {
			t.Fatalf("modify with uri %q: expected 200, got %d: %s", uri, w.Code, w.Body.String())
		}
		if !wantOK && w.Code != http.StatusBadRequest {
			t.Fatalf("modify with uri %q: expected 400, got %d", uri, w.Code)
		}
	}

	// 5. Arbitrary untrusted PPD file path (/tmp/evil.ppd)
	badPPDBody := `{"name":"Test_Queue_PPD","description":"Exploit","uri":"socket://192.168.1.99:9100","ppd_path":"/tmp/malicious.ppd"}`
	reqBadPPD := newTestReq(srv, "POST", "/api/v1/printers", strings.NewReader(badPPDBody))
	reqBadPPD.Header.Set("Content-Type", "application/json")
	wBadPPD := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBadPPD, reqBadPPD)

	if wBadPPD.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for untrusted PPD path in /tmp, got %d", wBadPPD.Code)
	}

	// 6. Arbitrary untrusted PPD driver swap on existing queue
	swapBadBody := `{"driver":"/tmp/evil.ppd"}`
	reqSwapBad := newTestReq(srv, "PUT", "/api/v1/printers/Office_LaserJet_Pro/driver", strings.NewReader(swapBadBody))
	reqSwapBad.Header.Set("Content-Type", "application/json")
	wSwapBad := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wSwapBad, reqSwapBad)

	if wSwapBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for untrusted PPD driver swap, got %d", wSwapBad.Code)
	}

	// 7. Malicious option key (CLI flag injection)
	badOptBody := `{"options":{"-o":"bad_value"}}`
	reqBadOpt := newTestReq(srv, "PUT", "/api/v1/printers/Office_LaserJet_Pro/options", strings.NewReader(badOptBody))
	reqBadOpt.Header.Set("Content-Type", "application/json")
	wBadOpt := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wBadOpt, reqBadOpt)

	if wBadOpt.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for flag injection in options, got %d", wBadOpt.Code)
	}
}

func TestAuthStatusEndpoint(t *testing.T) {
	srv, _ := setupTestServer()

	// Check status without token
	reqUnauth := httptest.NewRequest("GET", "/api/v1/auth/status", nil)
	reqUnauth.Host = "127.0.0.1:6310"
	wUnauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wUnauth, reqUnauth)

	if wUnauth.Code != http.StatusOK {
		t.Fatalf("expected 200 for auth status, got %d", wUnauth.Code)
	}
	var resUnauth map[string]any
	_ = json.Unmarshal(wUnauth.Body.Bytes(), &resUnauth)
	if resUnauth["authenticated"] != false {
		t.Errorf("expected authenticated=false without token, got: %v", resUnauth["authenticated"])
	}

	// Check status with valid token header
	reqAuth := newTestReq(srv, "GET", "/api/v1/auth/status", nil)
	wAuth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wAuth, reqAuth)

	var resAuth map[string]any
	_ = json.Unmarshal(wAuth.Body.Bytes(), &resAuth)
	if resAuth["authenticated"] != true {
		t.Errorf("expected authenticated=true with token, got: %v", resAuth["authenticated"])
	}
}

func TestSecurityDirectNavigationCookie(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Direct browser navigation to localhost:6310 root sets session cookie
	reqDirect := httptest.NewRequest("GET", "/", nil)
	reqDirect.Host = "127.0.0.1:6310"
	wDirect := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDirect, reqDirect)

	cookieHeader := wDirect.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, SessionCookieName) || !strings.Contains(cookieHeader, "SameSite=Strict") || !strings.Contains(cookieHeader, "HttpOnly") {
		t.Fatalf("expected Set-Cookie with SameSite=Strict and HttpOnly on direct loopback navigation, got: %q", cookieHeader)
	}

	// 2. Cross-site navigation (Sec-Fetch-Site: cross-site) must NOT receive session cookie
	reqCross := httptest.NewRequest("GET", "/", nil)
	reqCross.Host = "127.0.0.1:6310"
	reqCross.Header.Set("Sec-Fetch-Site", "cross-site")
	wCross := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wCross, reqCross)

	if strings.Contains(wCross.Header().Get("Set-Cookie"), SessionCookieName) {
		t.Fatalf("cross-site request must NOT receive session cookie")
	}
}

func TestSecurityAuthSessionEndpoint(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Valid loopback request to /api/v1/auth/session establishes session
	req := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	req.Host = "127.0.0.1:6310"
	req.Header.Set("X-Requested-With", "CUPS-WebUI")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for session auth, got %d", w.Code)
	}
	var res map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["authenticated"] != true || res["token"] != testSecretToken {
		t.Errorf("unexpected auth session response: %+v", res)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), SessionCookieName) {
		t.Errorf("expected Set-Cookie on auth session endpoint")
	}

	// 2. Cross-site request to /api/v1/auth/session is rejected with 403 Forbidden
	reqCross := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	reqCross.Host = "127.0.0.1:6310"
	reqCross.Header.Set("Sec-Fetch-Site", "cross-site")
	wCross := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wCross, reqCross)

	if wCross.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-site auth session request, got %d", wCross.Code)
	}

	// 3. Unauthorized Origin is rejected with 403 Forbidden
	reqOrigin := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	reqOrigin.Host = "127.0.0.1:6310"
	reqOrigin.Header.Set("Origin", "https://evil-attacker.com")
	wOrigin := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wOrigin, reqOrigin)

	if wOrigin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for external origin, got %d", wOrigin.Code)
	}
}

func TestSecurityJobIDFlagInjection(t *testing.T) {
	srv, _ := setupTestServer()

	// Try injecting flag "-a" (cancel all)
	req := httptest.NewRequest("POST", "/api/v1/jobs/-a/cancel", nil)
	req.Host = "127.0.0.1:6310"
	req.Header.Set("X-CUPS-WebUI-Token", testSecretToken)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malicious job ID '-a', got %d", w.Code)
	}

	// Try injecting flag "-u"
	req2 := httptest.NewRequest("POST", "/api/v1/jobs/-u%20root/hold", nil)
	req2.Host = "127.0.0.1:6310"
	req2.Header.Set("X-CUPS-WebUI-Token", testSecretToken)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malicious job ID, got %d", w2.Code)
	}
}

func TestSecuritySSENoWildcardCORS(t *testing.T) {
	srv, _ := setupTestServer()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so SSE handler sets headers and exits select loop
	req := httptest.NewRequest("GET", "/api/v1/events", nil).WithContext(ctx)
	req.Host = "127.0.0.1:6310"
	req.Header.Set("Origin", "https://evil-attacker.com")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Cross-origin request from attacker site must NOT receive Access-Control-Allow-Origin: * or attacker origin
	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader == "*" || corsHeader == "https://evil-attacker.com" {
		t.Fatalf("SSE stream leaked wildcard CORS or attacker origin: %q", corsHeader)
	}
}

// TestSecurityEmptyTokenFailsClosed ensures that a server without a configured
// secret never authenticates any request (fail closed, not fail open).
func TestSecurityEmptyTokenFailsClosed(t *testing.T) {
	srv, _ := setupTestServer()
	srv.token = "" // simulate a misconfigured server

	// 1. Mutating request with no token -> 401
	req := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	req.Host = "127.0.0.1:6310"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with empty server token and no client token, got %d", w.Code)
	}

	// 2. Mutating request with an empty token header and empty cookie -> 401
	reqEmpty := httptest.NewRequest("POST", "/api/v1/printers/Office_LaserJet_Pro/pause", nil)
	reqEmpty.Host = "127.0.0.1:6310"
	reqEmpty.Header.Set("X-CUPS-WebUI-Token", "")
	reqEmpty.Header.Set("Authorization", "Bearer ")
	reqEmpty.AddCookie(&http.Cookie{Name: SessionCookieName, Value: ""})
	wEmpty := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with empty server token and empty client credentials, got %d", wEmpty.Code)
	}

	// 3. Auth status reports not authenticated
	reqStatus := httptest.NewRequest("GET", "/api/v1/auth/status", nil)
	reqStatus.Host = "127.0.0.1:6310"
	wStatus := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wStatus, reqStatus)
	var status map[string]any
	_ = json.Unmarshal(wStatus.Body.Bytes(), &status)
	if status["authenticated"] != false {
		t.Errorf("expected authenticated=false with empty server token, got %+v", status)
	}

	// 4. Session bootstrap refuses to claim success
	reqSession := httptest.NewRequest("POST", "/api/v1/auth/session", nil)
	reqSession.Host = "127.0.0.1:6310"
	reqSession.Header.Set("X-Requested-With", "CUPS-WebUI")
	wSession := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wSession, reqSession)
	if wSession.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from auth session with empty server token, got %d", wSession.Code)
	}
	if strings.Contains(wSession.Body.String(), `"authenticated":true`) {
		t.Errorf("auth session must not report authenticated without a server token")
	}
}

// TestSetTokenIgnoresEmpty ensures the server token cannot be cleared.
func TestSetTokenIgnoresEmpty(t *testing.T) {
	srv, _ := setupTestServer()
	srv.SetToken("")
	if srv.Token() != testSecretToken {
		t.Fatalf("expected token to remain %q after SetToken(\"\"), got %q", testSecretToken, srv.Token())
	}
}

// Saving a queue with an emptied description or location must clear it, while
// fields left out of the request must keep their current values.
func TestModifyPrinterClearsAndPreservesFields(t *testing.T) {
	srv, _ := setupTestServer()
	const queue = "Office_LaserJet_Pro"

	put := func(body string) {
		t.Helper()
		req := newTestReq(srv, "PUT", "/api/v1/printers/"+queue, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT %s: expected 200, got %d: %s", body, w.Code, w.Body.String())
		}
	}
	get := func() cups.Printer {
		t.Helper()
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, newTestReq(srv, "GET", "/api/v1/printers/"+queue, nil))
		var p cups.Printer
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatalf("decoding printer: %v", err)
		}
		return p
	}

	put(`{"description":"Front desk","location":"Level 2","shared":true}`)
	if p := get(); p.Info != "Front desk" || p.Location != "Level 2" || !p.IsShared {
		t.Fatalf("setup save not applied: %+v", p)
	}
	uri := get().URI

	// Emptied fields are cleared
	put(`{"description":"","location":"","shared":true}`)
	if p := get(); p.Info != "" || p.Location != "" {
		t.Fatalf("expected description and location to be cleared, got info=%q location=%q", p.Info, p.Location)
	}

	// Fields left out are preserved, including sharing and the device URI
	put(`{"description":"Front desk","location":"Level 2","shared":true}`)
	put(`{"location":"Level 3"}`)
	p := get()
	if p.Info != "Front desk" || p.Location != "Level 3" || !p.IsShared || p.URI != uri {
		t.Fatalf("omitted fields must be preserved, got %+v", p)
	}

	// Sharing is only turned off when asked
	put(`{"shared":false}`)
	if p := get(); p.IsShared || p.Info != "Front desk" || p.Location != "Level 3" {
		t.Fatalf("expected only sharing to change, got %+v", p)
	}
}
