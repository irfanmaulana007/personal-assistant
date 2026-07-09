package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// handleListPricing returns the effective per-model rates (built-ins overlaid
// with DB overrides).
func (s *Server) handleListPricing(w http.ResponseWriter, r *http.Request) {
	prices := s.pricing.Effective(r.Context())
	sort.Slice(prices, func(i, j int) bool { return prices[i].Model < prices[j].Model })
	writeJSON(w, http.StatusOK, prices)
}

type modelPriceReq struct {
	Model       string  `json:"model"`
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

// handleSetPricing upserts a per-model rate override.
func (s *Server) handleSetPricing(w http.ResponseWriter, r *http.Request) {
	var req modelPriceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}
	if req.InputPer1M < 0 || req.OutputPer1M < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rates must be zero or positive"})
		return
	}
	if err := s.store.UpsertModelPrice(r.Context(), store.ModelPrice{
		Model: req.Model, InputPer1M: req.InputPer1M, OutputPer1M: req.OutputPer1M,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save price"})
		return
	}
	_ = s.pricing.Reload(r.Context())
	s.handleListPricing(w, r)
}

// handleDeletePricing removes a per-model override (reverting to built-in/unpriced).
func (s *Server) handleDeletePricing(w http.ResponseWriter, r *http.Request) {
	model := strings.TrimSpace(r.PathValue("model"))
	if model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}
	if err := s.store.DeleteModelPrice(r.Context(), model); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete price"})
		return
	}
	_ = s.pricing.Reload(r.Context())
	s.handleListPricing(w, r)
}
