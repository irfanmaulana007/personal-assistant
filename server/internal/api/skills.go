package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type skillResp struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Enabled     bool   `json:"enabled"`
	// Prompt management fields. Populated only for admins, since the prompt is
	// internal behaviour a member never needs and only admins may edit.
	Prompt          string  `json:"prompt,omitempty"`
	DefaultPrompt   string  `json:"default_prompt,omitempty"`
	PromptUpdatedAt *string `json:"prompt_updated_at,omitempty"`
	PromptUpdatedBy string  `json:"prompt_updated_by,omitempty"`
}

// handleListSkills returns the current user's skills with effective enabled
// state. For admins each skill additionally carries its editable prompt, the
// code-owned default, and who last edited it.
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
			resp.DefaultPrompt = store.DefaultSkillPrompt(u.Key)
			resp.PromptUpdatedBy = u.PromptUpdatedBy
			if u.PromptUpdatedAt != nil {
				ts := u.PromptUpdatedAt.Format(time.RFC3339)
				resp.PromptUpdatedAt = &ts
			}
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetSkillPrompt updates a skill's prompt (admin only). A "reset" request
// restores the code-owned default; otherwise the supplied prompt is saved and
// stamped with the editing admin's email and the current time.
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
		Reset  bool   `json:"reset"`
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

	if req.Reset {
		// Reset to the shipped default and hand the prompt back to the boot seed
		// (empty updatedBy clears the customization stamp).
		if err := s.store.SetSkillPrompt(r.Context(), id, store.DefaultSkillPrompt(sk.Key), ""); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
			return
		}
		s.handleListSkills(w, r)
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt cannot be empty"})
		return
	}
	if err := s.store.SetSkillPrompt(r.Context(), id, prompt, claims.Email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
		return
	}
	s.handleListSkills(w, r)
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
