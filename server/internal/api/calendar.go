package api

import "net/http"

// handleClearCalendarEvents deletes every event on the user's Composio-connected
// Google Calendar(s). It exists to recover from a runaway flood of duplicate
// events (e.g. hundreds accidentally created by a misbehaving reconciler) that
// the agent can no longer clear on its own and that Google Calendar's own UI
// refuses to bulk-delete. Destructive and admin-only.
func (s *Server) handleClearCalendarEvents(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if s.calendar == nil || !s.calendar.HasCalendar(r.Context(), claims.UserID()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no Google Calendar is connected"})
		return
	}
	deleted, failed, err := s.calendar.ClearAll(r.Context(), claims.UserID())
	if err != nil {
		s.log.Error("clear calendar events", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear calendar events"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted, "failed": failed})
}
