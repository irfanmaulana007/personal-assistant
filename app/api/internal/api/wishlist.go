package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

type wishItemResp struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	EstimatedPrice int64  `json:"estimated_price"`
	BuyMonth       string `json:"buy_month"` // "YYYY-MM", or "" when undecided
	Priority       string `json:"priority"`  // low | medium | high
	Link           string `json:"link"`
	Note           string `json:"note"`
	Done           bool   `json:"done"`
	DoneAt         string `json:"done_at"` // RFC3339, or "" when not done
	Created        string `json:"created_at"`
}

type wishItemReq struct {
	Name           string `json:"name"`
	EstimatedPrice int64  `json:"estimated_price"`
	BuyMonth       string `json:"buy_month"`
	Priority       string `json:"priority"`
	Link           string `json:"link"`
	Note           string `json:"note"`
}

func toWishItemResp(w store.WishItem) wishItemResp {
	resp := wishItemResp{
		ID:             w.ID,
		Name:           w.Name,
		EstimatedPrice: w.EstimatedPrice,
		BuyMonth:       w.BuyMonth,
		Priority:       w.Priority,
		Link:           w.Link,
		Note:           w.Note,
		Done:           w.Done,
		Created:        w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if w.DoneAt != nil {
		resp.DoneAt = w.DoneAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// normalizeBuyMonth trims a target month and validates the "YYYY-MM" shape,
// returning "" for an empty value. A malformed value is rejected.
func normalizeBuyMonth(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if _, err := time.Parse("2006-01", s); err != nil {
		return "", false
	}
	return s, true
}

// handleListWishItems returns the current user's wishlist items.
func (s *Server) handleListWishItems(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := s.store.ListWishItems(r.Context(), claims.UserID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load wishlist"})
		return
	}
	out := make([]wishItemResp, 0, len(items))
	for _, it := range items {
		out = append(out, toWishItemResp(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateWishItem adds an item to the current user's wishlist.
func (s *Server) handleCreateWishItem(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req wishItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	buyMonth, ok := normalizeBuyMonth(req.BuyMonth)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "buy_month must be YYYY-MM"})
		return
	}
	it, err := s.store.CreateWishItem(r.Context(), claims.UserID(), name, req.EstimatedPrice, buyMonth, req.Priority, strings.TrimSpace(req.Link), strings.TrimSpace(req.Note))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create wish item"})
		return
	}
	writeJSON(w, http.StatusOK, toWishItemResp(*it))
}

// handleUpdateWishItem edits an item's fields.
func (s *Server) handleUpdateWishItem(w http.ResponseWriter, r *http.Request) {
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
	var req wishItemReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	buyMonth, ok := normalizeBuyMonth(req.BuyMonth)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "buy_month must be YYYY-MM"})
		return
	}
	if err := s.store.UpdateWishItem(r.Context(), claims.UserID(), id, name, req.EstimatedPrice, buyMonth, req.Priority, strings.TrimSpace(req.Link), strings.TrimSpace(req.Note)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update wish item"})
		return
	}
	it, err := s.store.GetWishItem(r.Context(), claims.UserID(), id)
	if err != nil || it == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "wish item not found"})
		return
	}
	writeJSON(w, http.StatusOK, toWishItemResp(*it))
}

// handleSetWishItemDone marks an item as purchased/checked out, or reopens it.
func (s *Server) handleSetWishItemDone(w http.ResponseWriter, r *http.Request) {
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
		// Optional purchase date. Accepts an RFC3339 timestamp or a plain
		// "YYYY-MM-DD" date; ignored when reopening. Defaults to now.
		DoneAt string `json:"done_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	var doneAt *time.Time
	if req.Done && strings.TrimSpace(req.DoneAt) != "" {
		t, perr := parseDoneAt(strings.TrimSpace(req.DoneAt))
		if perr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid done_at date"})
			return
		}
		doneAt = &t
	}
	if err := s.store.SetWishItemDone(r.Context(), claims.UserID(), id, req.Done, doneAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update wish item"})
		return
	}
	it, err := s.store.GetWishItem(r.Context(), claims.UserID(), id)
	if err != nil || it == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "wish item not found"})
		return
	}
	writeJSON(w, http.StatusOK, toWishItemResp(*it))
}

// handleDeleteWishItem removes an item from the wishlist.
func (s *Server) handleDeleteWishItem(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.DeleteWishItem(r.Context(), claims.UserID(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete wish item"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
