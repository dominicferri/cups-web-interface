package api

import (
	"encoding/json"
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
