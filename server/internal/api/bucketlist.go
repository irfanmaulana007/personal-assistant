package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type bucketItemResp struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Note           string `json:"note"`
	Category       string `json:"category"`
	ResolutionYear *int   `json:"resolution_year"` // null when not a resolution
	Done           bool   `json:"done"`
	DoneAt         string `json:"done_at"` // RFC3339, or "" when not done
	Created        string `json:"created_at"`
}

type bucketItemReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Note        string `json:"note"`
	Category    string `json:"category"`
}

func toBucketItemResp(g store.BucketItem) bucketItemResp {
	resp := bucketItemResp{
		ID:             g.ID,
		Title:          g.Title,
		Description:    g.Description,
		Note:           g.Note,
		Category:       g.Category,
		ResolutionYear: g.ResolutionYear,
		Done:           g.Done,
		Created:        g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if g.DoneAt != nil {
		resp.DoneAt = g.DoneAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// handleListBucketItems returns the current user's bucket-list items.
func (s *Server) handleListBucketItems(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := s.store.ListBucketItems(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load bucket list"})
		return
	}
	out := make([]bucketItemResp, 0, len(items))
	for _, g := range items {
		out = append(out, toBucketItemResp(g))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateBucketItem adds an item to the current user's bucket list.
func (s *Server) handleCreateBucketItem(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req bucketItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	g, err := s.store.CreateBucketItem(r.Context(), claims.UserID(), title, strings.TrimSpace(req.Description), strings.TrimSpace(req.Note), req.Category, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create bucket item"})
		return
	}
	writeJSON(w, http.StatusOK, toBucketItemResp(*g))
}

// handleUpdateBucketItem edits an item's title/description/note/category.
func (s *Server) handleUpdateBucketItem(w http.ResponseWriter, r *http.Request) {
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
	var req bucketItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if err := s.store.UpdateBucketItem(r.Context(), claims.UserID(), id, title, strings.TrimSpace(req.Description), strings.TrimSpace(req.Note), req.Category); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update bucket item"})
		return
	}
	g, err := s.store.GetBucketItem(r.Context(), claims.UserID(), id)
	if err != nil || g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bucket item not found"})
		return
	}
	writeJSON(w, http.StatusOK, toBucketItemResp(*g))
}

// handleSetBucketItemDone checks or unchecks an item.
func (s *Server) handleSetBucketItemDone(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.SetBucketItemDone(r.Context(), claims.UserID(), id, req.Done); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update bucket item"})
		return
	}
	g, err := s.store.GetBucketItem(r.Context(), claims.UserID(), id)
	if err != nil || g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bucket item not found"})
		return
	}
	writeJSON(w, http.StatusOK, toBucketItemResp(*g))
}

// handleSetBucketItemResolution flags an item as a resolution for a year, or
// clears the flag when "year" is null.
func (s *Server) handleSetBucketItemResolution(w http.ResponseWriter, r *http.Request) {
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
		Year *int `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Year != nil && (*req.Year < 1970 || *req.Year > 3000) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid year"})
		return
	}
	if err := s.store.SetBucketItemResolution(r.Context(), claims.UserID(), id, req.Year); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update resolution"})
		return
	}
	g, err := s.store.GetBucketItem(r.Context(), claims.UserID(), id)
	if err != nil || g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bucket item not found"})
		return
	}
	writeJSON(w, http.StatusOK, toBucketItemResp(*g))
}

// handleDeleteBucketItem removes an item from the bucket list.
func (s *Server) handleDeleteBucketItem(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.DeleteBucketItem(r.Context(), claims.UserID(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete bucket item"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
