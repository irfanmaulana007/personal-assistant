package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

type mappingResp struct {
	ID        int64  `json:"id"`
	JID       string `json:"jid"`
	Kind      string `json:"kind"`
	ProjectID int64  `json:"project_id"`
	Role      string `json:"role"`
	UserID    int64  `json:"user_id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
}

func toMappingResp(m store.WhatsAppMapping) mappingResp {
	return mappingResp{
		ID: m.ID, JID: m.JID, Kind: m.Kind, ProjectID: m.ProjectID, Role: m.Role,
		UserID: m.UserID, Label: m.Label, CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
}

type mappingReq struct {
	JID       string `json:"jid"`
	Kind      string `json:"kind"`
	ProjectID int64  `json:"project_id"`
	Role      string `json:"role"`
	UserID    int64  `json:"user_id"`
	Label     string `json:"label"`
}

// validateMapping normalizes and checks a mapping request. Group mappings may
// never grant superadmin (a group message must never confer full access); their
// role is clamped to admin/member.
func validateMapping(req *mappingReq) string {
	req.JID = strings.TrimSpace(req.JID)
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.JID == "" {
		return "jid is required"
	}
	if req.Kind != "group" && req.Kind != "personal" {
		return "kind must be group or personal"
	}
	if req.Role == "" {
		req.Role = store.ProjectRoleMember
	}
	switch req.Role {
	case store.GlobalRoleSuperadmin, store.ProjectRoleAdmin, store.ProjectRoleMember:
	default:
		return "role must be superadmin, admin, or member"
	}
	if req.Kind == "group" && req.Role == store.GlobalRoleSuperadmin {
		return "a group mapping cannot grant superadmin"
	}
	if req.ProjectID <= 0 {
		return "project_id is required"
	}
	return ""
}

func (s *Server) handleListWhatsAppMappings(w http.ResponseWriter, r *http.Request) {
	mappings, err := s.store.ListWhatsAppMappings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load mappings"})
		return
	}
	out := make([]mappingResp, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, toMappingResp(m))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateWhatsAppMapping(w http.ResponseWriter, r *http.Request) {
	var req mappingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if msg := validateMapping(&req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if p, err := s.store.GetProject(r.Context(), req.ProjectID); err != nil || p == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project not found"})
		return
	}
	m, err := s.store.CreateWhatsAppMapping(r.Context(), store.WhatsAppMapping{
		JID: req.JID, Kind: req.Kind, ProjectID: req.ProjectID, Role: req.Role,
		UserID: req.UserID, Label: req.Label,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create mapping"})
		return
	}
	writeJSON(w, http.StatusOK, toMappingResp(*m))
}

func (s *Server) handleUpdateWhatsAppMapping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mapping id"})
		return
	}
	var req mappingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if msg := validateMapping(&req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if err := s.store.UpdateWhatsAppMapping(r.Context(), id, store.WhatsAppMapping{
		JID: req.JID, Kind: req.Kind, ProjectID: req.ProjectID, Role: req.Role,
		UserID: req.UserID, Label: req.Label,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update mapping"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteWhatsAppMapping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mapping id"})
		return
	}
	if err := s.store.DeleteWhatsAppMapping(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete mapping"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
