package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
)

// writeAccessoryError maps accessory errors to HTTP status codes.
func writeAccessoryError(w http.ResponseWriter, err error) {
	var inputErr *cups.AccessoryInputError
	var verifyErr *cups.AccessoryVerifyError
	var historyErr *cups.AccessoryHistoryError
	switch {
	case errors.As(err, &historyErr):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, cups.ErrNotCUPSAdmin):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, cups.ErrAccessoriesUnavailable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &inputErr):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &verifyErr):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// handleGetAccessories returns a queue's installable options and where each value came from.
func (s *Server) handleGetAccessories(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}
	rep, err := s.adapter.GetAccessories(name)
	if err != nil {
		writeAccessoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleSetAccessories sets installable options manually. The client must
// confirm it showed the warning (acknowledged_warning: true).
func (s *Server) handleSetAccessories(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}
	var req struct {
		Values              map[string]string `json:"values"`
		AcknowledgedWarning bool              `json:"acknowledged_warning"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rep, err := s.adapter.SetAccessories(name, req.Values, req.AcknowledgedWarning)
	if rep != nil {
		s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "accessories_change", Target: name})
	}
	if err != nil {
		writeAccessoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleResetAccessories restores the values from before the manual changes.
func (s *Server) handleResetAccessories(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}
	rep, err := s.adapter.ResetAccessories(name)
	if err != nil {
		writeAccessoryError(w, err)
		return
	}
	s.events.Broadcast(SSEEvent{Type: "printer_update", Action: "accessories_reset", Target: name})
	writeJSON(w, http.StatusOK, rep)
}

// handleSuggestAccessories asks the device over IPP and returns suggestions only.
func (s *Server) handleSuggestAccessories(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !cups.IsValidPrinterName(name) {
		writeError(w, http.StatusBadRequest, "invalid printer name")
		return
	}
	res, err := s.adapter.SuggestAccessories(name)
	if err != nil {
		writeAccessoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
