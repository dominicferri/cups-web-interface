package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
)

func TestAccessoryEndpoints(t *testing.T) {
	srv, _ := setupTestServer()
	do := func(req *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	w := do(newTestReq(srv, "GET", "/api/v1/printers/Finance_Secure_Copier/accessories", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	var rep cups.AccessoryReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil || !rep.Available || !rep.IsAdmin || len(rep.Options) == 0 {
		t.Fatalf("report: %+v %v", rep, err)
	}
	opt := rep.Options[0]
	var other string
	for _, c := range opt.Choices {
		if c.Name != opt.Value {
			other = c.Name
		}
	}

	put := func(body string) *httptest.ResponseRecorder {
		req := newTestReq(srv, "PUT", "/api/v1/printers/Finance_Secure_Copier/accessories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		return do(req)
	}

	// The warning must be acknowledged.
	if w := put(`{"values":{"` + opt.Key + `":"` + other + `"}}`); w.Code != http.StatusBadRequest {
		t.Errorf("unacknowledged: %d", w.Code)
	}
	if w := put(`{"values":{"` + opt.Key + `":"Nope"},"acknowledged_warning":true}`); w.Code != http.StatusBadRequest {
		t.Errorf("invalid choice: %d", w.Code)
	}
	if w := put(`not json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", w.Code)
	}

	w = put(`{"values":{"` + opt.Key + `":"` + other + `"},"acknowledged_warning":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rep)
	if rep.Options[0].Source != cups.AccessorySourceManual || rep.Options[0].Override == nil {
		t.Errorf("after PUT: %+v", rep.Options[0])
	}

	w = do(newTestReq(srv, "POST", "/api/v1/printers/Finance_Secure_Copier/accessories/reset", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rep)
	if rep.HasOverrides || rep.Options[0].Value != opt.Value {
		t.Errorf("after reset: %+v", rep.Options[0])
	}

	w = do(newTestReq(srv, "POST", "/api/v1/printers/Finance_Secure_Copier/accessories/suggestions", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"suggestions":[]`) {
		t.Errorf("suggestions: %d %s", w.Code, w.Body.String())
	}

	// Driverless queue: report says unavailable, PUT is a conflict.
	w = do(newTestReq(srv, "GET", "/api/v1/printers/Warehouse_Zebra_Labels/accessories", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"unavailable_reason":"driverless"`) {
		t.Errorf("driverless GET: %d %s", w.Code, w.Body.String())
	}
	req := newTestReq(srv, "PUT", "/api/v1/printers/Warehouse_Zebra_Labels/accessories", strings.NewReader(`{"values":{"X":"Y"},"acknowledged_warning":true}`))
	if w := do(req); w.Code != http.StatusConflict {
		t.Errorf("driverless PUT: %d", w.Code)
	}

	if w := do(newTestReq(srv, "GET", "/api/v1/printers/No_Such_Queue/accessories", nil)); w.Code != http.StatusInternalServerError {
		t.Errorf("missing queue: %d", w.Code)
	}
}

func TestAccessoryEndpointsRequireToken(t *testing.T) {
	srv, _ := setupTestServer()
	for _, c := range []struct{ method, path, body string }{
		{"PUT", "/api/v1/printers/Finance_Secure_Copier/accessories", `{"values":{"HDD":"False"},"acknowledged_warning":true}`},
		{"POST", "/api/v1/printers/Finance_Secure_Copier/accessories/reset", ""},
		{"POST", "/api/v1/printers/Finance_Secure_Copier/accessories/suggestions", ""},
		{"POST", "/api/v1/printers/Finance_Secure_Copier/accessories/snmp", `{"host":"10.1.2.3","community":"public"}`},
		{"PUT", "/api/v1/accessories/features", `{"snmp_enabled":true}`},
		{"POST", "/api/v1/accessories/inventory", ""},
		{"DELETE", "/api/v1/accessories/inventory", ""},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Host = "127.0.0.1:6310"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: %d", c.method, c.path, w.Code)
		}
		// Cross-origin with the token is still refused.
		req = newTestReq(srv, c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Origin", "http://127.0.0.1:8080")
		w = httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s cross-origin: %d", c.method, c.path, w.Code)
		}
	}
}

func TestSNMPCheckEndpoint(t *testing.T) {
	srv, _ := setupTestServer()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := newTestReq(srv, method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	const snmp = "/api/v1/printers/Finance_Secure_Copier/accessories/snmp"
	const ok = `{"host":"10.1.2.3","community":"public"}`

	// Off by default.
	if w := do("POST", snmp, ok); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "turned off") {
		t.Fatalf("disabled: %d %s", w.Code, w.Body.String())
	}
	w := do("GET", "/api/v1/accessories/features", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"snmp_enabled":false`) {
		t.Fatalf("features: %d %s", w.Code, w.Body.String())
	}
	if w := do("PUT", "/api/v1/accessories/features", `{"snmp_enabled":true}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"snmp_enabled":true`) {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	if w := do("PUT", "/api/v1/accessories/features", `nope`); w.Code != http.StatusBadRequest {
		t.Errorf("bad features body: %d", w.Code)
	}

	if w := do("POST", snmp, `{"host":"-c","community":"public"}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad host: %d %s", w.Code, w.Body.String())
	}
	if w := do("POST", snmp, `{"host":"10.1.2.3:161","community":"public"}`); w.Code != http.StatusBadRequest {
		t.Errorf("host with port: %d", w.Code)
	}
	if w := do("POST", snmp, `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", w.Code)
	}

	w = do("POST", snmp, ok)
	if w.Code != http.StatusOK {
		t.Fatalf("check: %d %s", w.Code, w.Body.String())
	}
	var inv cups.SNMPInventory
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil || !inv.Queried || len(inv.Inputs) == 0 || len(inv.Finishers) == 0 {
		t.Errorf("inventory: %+v %v", inv, err)
	}
	if strings.Contains(w.Body.String(), "public") {
		t.Error("the response must not echo the community name")
	}
	if w := do("GET", "/api/v1/printers/Finance_Secure_Copier/accessories", ""); !strings.Contains(w.Body.String(), `"snmp_enabled":true`) {
		t.Errorf("report: %s", w.Body.String())
	}
}

func TestPrinterInventoryEndpoints(t *testing.T) {
	srv, _ := setupTestServer()
	upload := func(field, name, body string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile(field, name)
		_, _ = fw.Write([]byte(body))
		_ = mw.Close()
		req := newTestReq(srv, "POST", "/api/v1/accessories/inventory", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, newTestReq(srv, method, path, nil))
		return w
	}
	csv := "\"PrinterName\",\"Model\",\"Driver\",\"PortName\",\"IPAddress\"\n" +
		"\"Finance Secure Copier\",\"Example\",\"Example PS\",\"192.0.2.20\",\"192.0.2.20\"\n" +
		"\"Kiosk\",\"\",\"\",\"TS001\",\"N/A\"\n"

	if w := do("GET", "/api/v1/accessories/inventory"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"loaded":false`) {
		t.Fatalf("empty: %d %s", w.Code, w.Body.String())
	}
	w := upload("csv", "list.csv", csv)
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	var res cups.InventoryImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Count != 1 || len(res.Skipped) != 1 || res.Skipped[0].Line != 3 || res.Matched != 1 {
		t.Errorf("result: %+v %v", res, err)
	}

	// This mock queue has a direct device URI, so its host wins over the list
	// (the print server case is covered in the cups package tests).
	req := newTestReq(srv, "PUT", "/api/v1/accessories/features", strings.NewReader(`{"snmp_enabled":true}`))
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if w := do("GET", "/api/v1/printers/Finance_Secure_Copier/accessories"); !strings.Contains(w.Body.String(), `"snmp_host_source":"device_uri"`) || strings.Contains(w.Body.String(), "print_server_entry") {
		t.Errorf("report: %s", w.Body.String())
	}

	if w := upload("csv", "bad.csv", "Name,Address\nA,B\n"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "no PrinterName") {
		t.Errorf("bad header: %d %s", w.Code, w.Body.String())
	}
	if w := upload("file", "list.csv", csv); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "csv") {
		t.Errorf("wrong field: %d %s", w.Code, w.Body.String())
	}
	if w := upload("csv", "big.csv", csv+strings.Repeat("x", 3<<20)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("too big: %d %s", w.Code, w.Body.String())
	}
	req = newTestReq(srv, "POST", "/api/v1/accessories/inventory", strings.NewReader(csv))
	req.Header.Set("Content-Type", "text/csv")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("not multipart: %d", w.Code)
	}
	// Failed uploads keep the list.
	if w := do("GET", "/api/v1/accessories/inventory"); !strings.Contains(w.Body.String(), `"count":1`) {
		t.Errorf("after failures: %s", w.Body.String())
	}
	if w := do("DELETE", "/api/v1/accessories/inventory"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"loaded":false`) {
		t.Errorf("delete: %d %s", w.Code, w.Body.String())
	}
}
