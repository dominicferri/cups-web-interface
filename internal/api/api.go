package api

import (
	"encoding/json"
	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

// Server handles HTTP requests for CUPS webui.
type Server struct {
	adapter  cups.Adapter
	mux      *http.ServeMux
	staticFS fs.FS
	events   *EventBroadcaster
	token    string

	// peerIsSameUser reports whether a request's connection is owned by the
	// user running the server. Replaced in tests.
	peerIsSameUser func(*http.Request) bool
}

// NewServer creates a new API server with optional sessionToken.
func NewServer(adapter cups.Adapter, staticFS fs.FS, sessionToken ...string) *Server {
	var token string
	if len(sessionToken) > 0 && sessionToken[0] != "" {
		token = sessionToken[0]
	} else {
		token = GenerateSessionToken()
	}

	s := &Server{
		adapter:  adapter,
		mux:      http.NewServeMux(),
		staticFS: staticFS,
		events:   NewEventBroadcaster(),
		token:    token,

		peerIsSameUser: connectionIsSameUser,
	}
	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler with security middleware applied.
func (s *Server) Handler() http.Handler {
	return s.securityMiddleware(s.mux)
}

func (s *Server) registerRoutes() {
	// Authentication
	s.mux.HandleFunc("GET /api/v1/auth/status", s.handleAuthStatus)
	s.mux.HandleFunc("POST /api/v1/auth/session", s.handleAuthSession)

	// API v1 routes
	s.mux.HandleFunc("GET /api/v1/system/status", s.handleSystemStatus)
	s.mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/v1/printers", s.handleGetPrinters)
	s.mux.HandleFunc("POST /api/v1/printers", s.handleAddPrinter)
	s.mux.HandleFunc("GET /api/v1/printers/{name}", s.handleGetPrinter)
	s.mux.HandleFunc("PUT /api/v1/printers/{name}", s.handleModifyPrinter)
	s.mux.HandleFunc("PUT /api/v1/printers/{name}/driver", s.handleSetPrinterDriver)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/test-page", s.handlePrintTestPage)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/pause", s.handlePausePrinter)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/resume", s.handleResumePrinter)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/default", s.handleSetDefaultPrinter)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/purge", s.handlePurgePrinter)
	s.mux.HandleFunc("DELETE /api/v1/printers/{name}", s.handleDeletePrinter)
	s.mux.HandleFunc("GET /api/v1/jobs", s.handleGetJobs)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.handleCancelJob)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/hold", s.handleHoldJob)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/release", s.handleReleaseJob)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/restart", s.handleRestartJob)
	s.mux.HandleFunc("GET /api/v1/printers/{name}/options", s.handleGetPrinterOptions)
	s.mux.HandleFunc("PUT /api/v1/printers/{name}/options", s.handleSetPrinterOptions)
	s.mux.HandleFunc("GET /api/v1/printers/{name}/accessories", s.handleGetAccessories)
	s.mux.HandleFunc("PUT /api/v1/printers/{name}/accessories", s.handleSetAccessories)
	s.mux.HandleFunc("POST /api/v1/printers/{name}/accessories/reset", s.handleResetAccessories)
	// POST: it makes the server contact the device, so it needs the session token.
	s.mux.HandleFunc("POST /api/v1/printers/{name}/accessories/suggestions", s.handleSuggestAccessories)
	s.mux.HandleFunc("GET /api/v1/printers/{name}/audit", s.handleGetPrinterAudit)
	s.mux.HandleFunc("GET /api/v1/audit/summary", s.handleGetFleetAudit)
	s.mux.HandleFunc("GET /api/v1/drivers", s.handleGetDrivers)
	s.mux.HandleFunc("POST /api/v1/ppds/validate", s.handleValidatePPD)
	s.mux.HandleFunc("GET /api/v1/server/settings", s.handleGetServerSettings)
	s.mux.HandleFunc("PUT /api/v1/server/settings", s.handleSetServerSettings)
	s.mux.HandleFunc("GET /api/v1/system/logs", s.handleGetSystemLogs)
	s.mux.HandleFunc("GET /api/v1/system/launchagent", s.handleGetLaunchAgent)
	s.mux.HandleFunc("POST /api/v1/system/launchagent", s.handleManageLaunchAgent)

	// Static assets and SPA fallback
	if s.staticFS != nil {
		fileServer := http.FileServer(http.FS(s.staticFS))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// If request path is an API endpoint, do not serve SPA
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}

			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}

			// Check if file exists in embedded staticFS
			f, err := s.staticFS.Open(path)
			if err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// Fallback to index.html for client-side routing
			indexFile, err := s.staticFS.Open("index.html")
			if err == nil {
				_ = indexFile.Close()
				r.URL.Path = "/index.html"
				fileServer.ServeHTTP(w, r)
				return
			}

			http.NotFound(w, r)
		})
	}
}

// Helper JSON responses
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("error encoding response json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}
