package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIRefusesRequestsFromOtherSites(t *testing.T) {
	srv, _ := setupTestServer()
	paths := []struct{ method, path string }{
		{"GET", "/api/v1/printers"},
		{"GET", "/api/v1/printers/Finance_Secure_Copier/accessories"},
		{"GET", "/api/v1/events"},
		{"GET", "/api/v1/auth/status"},
		{"POST", "/api/v1/auth/session"},
		{"POST", "/api/v1/printers/Finance_Secure_Copier/accessories/suggestions"},
	}
	for _, p := range paths {
		for _, site := range []string{"cross-site", "same-site"} {
			req := newTestReq(srv, p.method, p.path, nil)
			req.Header.Set("Sec-Fetch-Site", site)
			req.Header.Set("X-Requested-With", "CUPS-WebUI")
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s with Sec-Fetch-Site %s: got %d, want 403", p.method, p.path, site, w.Code)
			}
		}
	}
	// The app's own pages, typed URLs and non-browser clients still work.
	for _, site := range []string{"same-origin", "none", ""} {
		req := newTestReq(srv, "GET", "/api/v1/printers/Finance_Secure_Copier/accessories", nil)
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Sec-Fetch-Site %q: got %d, want 200", site, w.Code)
		}
	}
}
