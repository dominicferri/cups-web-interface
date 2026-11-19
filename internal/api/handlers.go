package api

import (
	"encoding/json"
	"fmt"
	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.adapter.GetSystemStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGetPrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := s.adapter.GetPrinters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if printers == nil {
		printers = []cups.Printer{}
	}
	writeJSON(w, http.StatusOK, printers)
}

func (s *Server) handleGetPrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	printer, err := s.adapter.GetPrinter(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, printer)
}

func (s *Server) handlePausePrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	if err := s.adapter.PausePrinter(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "pause", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer paused successfully",
	})
}

func (s *Server) handleResumePrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	if err := s.adapter.ResumePrinter(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "resume", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer resumed successfully",
	})
}

func (s *Server) handleSetDefaultPrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	if err := s.adapter.SetDefaultPrinter(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "default", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "default printer updated successfully",
	})
}

func (s *Server) handlePurgePrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	if err := s.adapter.PurgeQueue(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "purge", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer queue purged successfully",
	})
}

func (s *Server) handleDeletePrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	if err := s.adapter.DeletePrinter(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "delete", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer queue deleted successfully",
	})
}

func (s *Server) handleGetJobs(w http.ResponseWriter, r *http.Request) {
	which := r.URL.Query().Get("which")
	if which == "" {
		which = "active"
	}
	printer := r.URL.Query().Get("printer")

	jobs, err := s.adapter.GetJobs(which, printer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []cups.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cups.IsValidJobID(id) {
		writeError(w, http.StatusBadRequest, "invalid job ID format")
		return
	}

	if err := s.adapter.CancelJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "job_update", Action: "cancel", Target: id})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "job cancelled successfully",
	})
}

func (s *Server) handleHoldJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cups.IsValidJobID(id) {
		writeError(w, http.StatusBadRequest, "invalid job ID format")
		return
	}

	if err := s.adapter.HoldJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "job_update", Action: "hold", Target: id})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "job held successfully",
	})
}

func (s *Server) handleReleaseJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cups.IsValidJobID(id) {
		writeError(w, http.StatusBadRequest, "invalid job ID format")
		return
	}

	if err := s.adapter.ReleaseJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "job_update", Action: "release", Target: id})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "job released successfully",
	})
}

func (s *Server) handleRestartJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cups.IsValidJobID(id) {
		writeError(w, http.StatusBadRequest, "invalid job ID format")
		return
	}

	if err := s.adapter.RestartJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.events.Broadcast(SSEEvent{Type: "job_update", Action: "restart", Target: id})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "job restarted successfully",
	})
}

// handleGetPrinterOptions returns configurable options for a printer.
func (s *Server) handleGetPrinterOptions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "printer name is required")
		return
	}

	opts, err := s.adapter.GetPrinterOptions(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// handleSetPrinterOptions updates options for a printer.
func (s *Server) handleSetPrinterOptions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	var req struct {
		Options map[string]string `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for k, v := range req.Options {
		if !cups.IsValidOptionKey(k) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid option key %q", k))
			return
		}
		if !cups.IsValidOptionValue(v) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid option value for key %q", k))
			return
		}
	}

	if err := s.adapter.SetPrinterOptions(name, req.Options); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "options_change", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer options updated successfully",
	})
}

// handleGetPrinterAudit returns detailed binary slices and Rosetta 2 audit for a printer.
func (s *Server) handleGetPrinterAudit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	audit, err := s.adapter.GetPrinterAudit(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, audit)
}

// handleGetFleetAudit returns global Rosetta 2 readiness summary across all queues.
func (s *Server) handleGetFleetAudit(w http.ResponseWriter, r *http.Request) {
	summary, err := s.adapter.GetFleetAudit()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleAddPrinter(w http.ResponseWriter, r *http.Request) {
	var params cups.AddPrinterParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(params.Name)
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name: max 127 characters, must not start with '-', and no spaces, control characters or / \\ ' \" ? #")
		return
	}

	// Prevent silent queue overwriting
	if existing, _ := s.adapter.GetPrinter(name); existing != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("a printer queue named %q already exists; choose a different name or edit the existing printer", name))
		return
	}

	if !cups.IsValidPrinterURI(params.URI) {
		writeError(w, http.StatusBadRequest, "invalid printer connection URI: use an ipp, ipps, http, https, socket, lpd, usb, smb, dnssd or mdns address (or file:/dev/null for a null queue) without control characters or malformed %-escapes")
		return
	}

	if params.PPDPath != "" && !cups.IsValidDriverOrPPD(params.PPDPath) {
		writeError(w, http.StatusBadRequest, "invalid or untrusted PPD file path")
		return
	}
	if params.Model != "" && !cups.IsValidDriverOrPPD(params.Model) {
		writeError(w, http.StatusBadRequest, "invalid or untrusted driver model identifier")
		return
	}

	if err := s.adapter.AddPrinter(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "create", Target: name})
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":  "ok",
		"message": "printer queue created successfully",
		"name":    name,
	})
}

func (s *Server) handleModifyPrinter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	var params cups.ModifyPrinterParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if params.URI != "" && !cups.IsValidPrinterURI(params.URI) {
		writeError(w, http.StatusBadRequest, "invalid printer connection URI: use an ipp, ipps, http, https, socket, lpd, usb, smb, dnssd or mdns address (or file:/dev/null for a null queue) without control characters or malformed %-escapes")
		return
	}

	if err := s.adapter.ModifyPrinter(name, params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "modify", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer updated successfully",
	})
}

func (s *Server) handleSetPrinterDriver(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	var req struct {
		Driver string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Driver) == "" {
		writeError(w, http.StatusBadRequest, "driver/ppd identifier is required")
		return
	}

	if !cups.IsValidDriverOrPPD(req.Driver) {
		writeError(w, http.StatusBadRequest, "invalid or untrusted driver model or PPD path: must be a system model or reside in /Library/Printers or /usr/share/cups/model")
		return
	}

	if err := s.adapter.SetPrinterDriver(name, req.Driver); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "driver_swap", Target: name})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "printer driver swapped successfully",
	})
}

func (s *Server) handlePrintTestPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}

	jobID, err := s.adapter.PrintTestPage(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "job_update", Action: "create", Target: name})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"message": "test page submitted successfully",
		"job_id":  jobID,
	})
}

func (s *Server) handleGetDrivers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	if search == "" {
		search = r.URL.Query().Get("make")
	}

	drivers, err := s.adapter.GetDrivers(search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if drivers == nil {
		drivers = []cups.DriverInfo{}
	}
	writeJSON(w, http.StatusOK, drivers)
}

func (s *Server) handleValidatePPD(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 10MB to prevent memory exhaustion DoS
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var content []byte
	var err error

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}
		file, _, fileErr := r.FormFile("ppd")
		if fileErr != nil {
			writeError(w, http.StatusBadRequest, "missing 'ppd' file in form data")
			return
		}
		defer file.Close()
		content, err = io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read uploaded file")
			return
		}
	} else {
		content, err = io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read request body")
			return
		}
	}

	res, err := s.adapter.ValidatePPD(content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetServerSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.adapter.GetServerSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleSetServerSettings(w http.ResponseWriter, r *http.Request) {
	var settings cups.ServerSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.adapter.SetServerSettings(settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "server_settings", Action: "update"})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "server settings updated successfully",
	})
}

func (s *Server) handleGetSystemLogs(w http.ResponseWriter, r *http.Request) {
	lines := 100
	if lStr := r.URL.Query().Get("lines"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			lines = parsed
			if lines > 1000 {
				lines = 1000
			}
		}
	}
	level := r.URL.Query().Get("level")
	if level == "" {
		level = "all"
	}

	logs, err := s.adapter.GetLogs(lines, level)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []cups.LogEntry{}
	}
	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handleGetLaunchAgent(w http.ResponseWriter, r *http.Request) {
	status, err := s.adapter.GetLaunchAgentStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleManageLaunchAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"` // "install" or "uninstall"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.adapter.ManageLaunchAgent(body.Action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.events.Broadcast(SSEEvent{Type: "launchagent", Action: body.Action})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("LaunchAgent %s action completed successfully", body.Action),
	})
}
