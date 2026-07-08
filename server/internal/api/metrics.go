package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
)

type usageSummaryResp struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type usageDayResp struct {
	Date        string `json:"date"`
	Requests    int    `json:"requests"`
	TotalTokens int    `json:"total_tokens"`
}

type usageModelResp struct {
	Model            string  `json:"model"`
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	RateKnown        bool    `json:"rate_known"`
}

type usageResp struct {
	RangeDays int              `json:"range_days"`
	Summary   usageSummaryResp `json:"summary"`
	ByDay     []usageDayResp   `json:"by_day"`
	ByModel   []usageModelResp `json:"by_model"`
	// CostEstimated is true when at least one model's cost could not be priced.
	CostPartial bool `json:"cost_partial"`
}

// handleMetricsUsage returns aggregated LLM usage and estimated cost.
// Query param: days (default 30, clamped 1..365).
func (s *Server) handleMetricsUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	stats, err := s.store.UsageStatsSince(r.Context(), since)
	if err != nil {
		s.log.Error("failed to load usage stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage"})
		return
	}

	resp := usageResp{
		RangeDays: days,
		ByDay:     make([]usageDayResp, 0, len(stats.ByDay)),
		ByModel:   make([]usageModelResp, 0, len(stats.ByModel)),
	}

	for _, d := range stats.ByDay {
		resp.ByDay = append(resp.ByDay, usageDayResp{Date: d.Date, Requests: d.Requests, TotalTokens: d.TotalTokens})
	}

	var totalCost float64
	for _, m := range stats.ByModel {
		cost, known := llm.EstimateCost(m.Model, m.PromptTokens, m.CompletionTokens)
		if known {
			totalCost += cost
		} else {
			resp.CostPartial = true
		}
		resp.ByModel = append(resp.ByModel, usageModelResp{
			Model:            m.Model,
			Requests:         m.Requests,
			PromptTokens:     m.PromptTokens,
			CompletionTokens: m.CompletionTokens,
			TotalTokens:      m.TotalTokens,
			EstimatedCostUSD: cost,
			RateKnown:        known,
		})
	}

	resp.Summary = usageSummaryResp{
		Requests:         stats.Summary.Requests,
		PromptTokens:     stats.Summary.PromptTokens,
		CompletionTokens: stats.Summary.CompletionTokens,
		TotalTokens:      stats.Summary.TotalTokens,
		EstimatedCostUSD: totalCost,
	}

	writeJSON(w, http.StatusOK, resp)
}
