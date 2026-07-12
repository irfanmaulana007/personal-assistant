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
	// Prompt is the DB-owned system-prompt fragment injected when the skill is
	// enabled. It is master data and only exposed to admins (who manage it from
	// the Skills page); it stays empty in responses to non-admin members.
	Prompt string `json:"prompt,omitempty"`
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
	isAdmin := claims.Role == "admin"
	out := make([]skillResp, 0, len(list))
	for _, u := range list {
		resp := skillResp{
			ID: u.ID, Key: u.Key, Name: u.Name, Description: u.Description, Category: u.Category, Enabled: u.Enabled,
		}
		if isAdmin {
			resp.Prompt = u.Prompt
		}
		out = append(out, resp)
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

// handleSetSkillPrompt updates a skill's system-prompt fragment. The prompt is
// global master data, so this route is admin-only (wired with the admin
// middleware). It returns the refreshed skill list on success.
func (s *Server) handleSetSkillPrompt(w http.ResponseWriter, r *http.Request) {
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
		Prompt string `json:"prompt"`
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

	if err := s.store.UpdateSkillPrompt(r.Context(), id, req.Prompt); err != nil {
		s.log.Error("update skill prompt", "error", err, "skill", sk.Key)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
		return
	}
	s.handleListSkills(w, r)
}
