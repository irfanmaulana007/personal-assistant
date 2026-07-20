package api

import (
	"encoding/json"
	"net/http"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/routine"
)

// handleListRoutines returns every daily routine's configuration (any user).
func (s *Server) handleListRoutines(w http.ResponseWriter, r *http.Request) {
	if s.routines == nil {
		writeJSON(w, http.StatusOK, []routine.View{})
		return
	}
	writeJSON(w, http.StatusOK, s.routines.List(r.Context()))
}

type routineUpdateReq struct {
	Enabled *bool   `json:"enabled"`
	Time    *string `json:"time"`
	Prompt  *string `json:"prompt"`
}

// handleUpdateRoutine applies a partial change to a routine (admin only).
func (s *Server) handleUpdateRoutine(w http.ResponseWriter, r *http.Request) {
	if s.routines == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "routines unavailable"})
		return
	}
	var req routineUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	view, err := s.routines.Update(r.Context(), r.PathValue("key"), routine.Update{
		Enabled: req.Enabled,
		Time:    req.Time,
		Prompt:  req.Prompt,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleRunRoutine runs a routine immediately, ignoring its schedule (admin
// only) — a "run now" affordance for testing.
func (s *Server) handleRunRoutine(w http.ResponseWriter, r *http.Request) {
	if s.routines == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "routines unavailable"})
		return
	}
	sent, message, err := s.routines.RunNow(r.Context(), r.PathValue("key"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "message": message})
}
