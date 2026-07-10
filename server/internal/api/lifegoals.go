package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type lifeGoalResp struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Note    string `json:"note"`
	Done    bool   `json:"done"`
	DoneAt  string `json:"done_at"` // RFC3339, or "" when not done
	Created string `json:"created_at"`
}

type lifeGoalReq struct {
	Title string `json:"title"`
	Note  string `json:"note"`
}

func toLifeGoalResp(g store.LifeGoal) lifeGoalResp {
	resp := lifeGoalResp{
		ID:      g.ID,
		Title:   g.Title,
		Note:    g.Note,
		Done:    g.Done,
		Created: g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if g.DoneAt != nil {
		resp.DoneAt = g.DoneAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// handleListLifeGoals returns the current user's life-list items.
func (s *Server) handleListLifeGoals(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	goals, err := s.store.ListLifeGoals(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load life goals"})
		return
	}
	out := make([]lifeGoalResp, 0, len(goals))
	for _, g := range goals {
		out = append(out, toLifeGoalResp(g))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateLifeGoal adds an item to the current user's life list.
func (s *Server) handleCreateLifeGoal(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req lifeGoalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	g, err := s.store.CreateLifeGoal(r.Context(), claims.UserID(), title, strings.TrimSpace(req.Note))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create life goal"})
		return
	}
	writeJSON(w, http.StatusOK, toLifeGoalResp(*g))
}

// handleUpdateLifeGoal edits an item's title/note.
func (s *Server) handleUpdateLifeGoal(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req lifeGoalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if err := s.store.UpdateLifeGoal(r.Context(), claims.UserID(), id, title, strings.TrimSpace(req.Note)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update life goal"})
		return
	}
	g, err := s.store.GetLifeGoal(r.Context(), claims.UserID(), id)
	if err != nil || g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "life goal not found"})
		return
	}
	writeJSON(w, http.StatusOK, toLifeGoalResp(*g))
}

// handleSetLifeGoalDone checks or unchecks an item.
func (s *Server) handleSetLifeGoalDone(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Done bool `json:"done"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.store.SetLifeGoalDone(r.Context(), claims.UserID(), id, req.Done); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update life goal"})
		return
	}
	g, err := s.store.GetLifeGoal(r.Context(), claims.UserID(), id)
	if err != nil || g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "life goal not found"})
		return
	}
	writeJSON(w, http.StatusOK, toLifeGoalResp(*g))
}

// handleDeleteLifeGoal removes an item from the life list.
func (s *Server) handleDeleteLifeGoal(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteLifeGoal(r.Context(), claims.UserID(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete life goal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
