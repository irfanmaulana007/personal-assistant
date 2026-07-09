package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type skillResp struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Enabled     bool   `json:"enabled"`
}

// handleListSkills returns the current user's skills with effective enabled state.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	list, err := s.store.ListUserSkills(r.Context(), claims.UserID())
	if err != nil {
		s.log.Error("list user skills", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	out := make([]skillResp, 0, len(list))
	for _, u := range list {
		out = append(out, skillResp{
			ID: u.ID, Key: u.Key, Name: u.Name, Description: u.Description, Category: u.Category, Enabled: u.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetSkill enables/disables a skill for the current user.
func (s *Server) handleSetSkill(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sk, err := s.store.GetSkill(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	if err := s.store.SetSkillEnabled(r.Context(), claims.UserID(), id, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
		return
	}
	s.handleListSkills(w, r)
}
