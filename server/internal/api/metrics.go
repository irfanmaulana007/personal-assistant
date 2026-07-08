package api

import (
	"net/http"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
)

type usageSummaryResp struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	AvgLatencyMs     int     `json:"avg_latency_ms"`
	ToolCalls        int     `json:"tool_calls"`
}

type usagePlatformResp struct {
	Platform    string `json:"platform"`
	Requests    int    `json:"requests"`
	TotalTokens int    `json:"total_tokens"`
}

type toolCountResp struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
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
	From       string              `json:"from"` // inclusive, YYYY-MM-DD
	To         string              `json:"to"`   // inclusive, YYYY-MM-DD
	Summary    usageSummaryResp    `json:"summary"`
	ByDay      []usageDayResp      `json:"by_day"`
	ByModel    []usageModelResp    `json:"by_model"`
	ByPlatform []usagePlatformResp `json:"by_platform"`
	TopTools   []toolCountResp     `json:"top_tools"`
	// CostPartial is true when at least one model's cost could not be priced.
	CostPartial bool `json:"cost_partial"`
}

const dateLayout = "2006-01-02"

// handleMetricsUsage returns aggregated LLM usage and estimated cost for an
// inclusive date range. Query params: from, to (YYYY-MM-DD, UTC). Defaults to
// the last 30 days. The range is clamped to 366 days.
func (s *Server) handleMetricsUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)

	to := today
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			to = t
		}
	}
	from := to.AddDate(0, 0, -29)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(dateLayout, v); err == nil {
			from = t
		}
	}

	if from.After(to) {
		from, to = to, from
	}
	// Clamp the range to 366 days.
	if to.Sub(from) > 366*24*time.Hour {
		from = to.AddDate(0, 0, -366)
	}

	// `to` is inclusive; query with an exclusive end at the start of the next day.
	toExclusive := to.AddDate(0, 0, 1)

	stats, err := s.store.UsageStatsBetween(r.Context(), from, toExclusive)
	if err != nil {
		s.log.Error("failed to load usage stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage"})
		return
	}

	resp := usageResp{
		From:       from.Format(dateLayout),
		To:         to.Format(dateLayout),
		ByDay:      make([]usageDayResp, 0, len(stats.ByDay)),
		ByModel:    make([]usageModelResp, 0, len(stats.ByModel)),
		ByPlatform: make([]usagePlatformResp, 0, len(stats.ByPlatform)),
		TopTools:   make([]toolCountResp, 0, len(stats.TopTools)),
	}

	for _, d := range stats.ByDay {
		resp.ByDay = append(resp.ByDay, usageDayResp{Date: d.Date, Requests: d.Requests, TotalTokens: d.TotalTokens})
	}
	for _, p := range stats.ByPlatform {
		resp.ByPlatform = append(resp.ByPlatform, usagePlatformResp{Platform: p.Platform, Requests: p.Requests, TotalTokens: p.TotalTokens})
	}
	for _, t := range stats.TopTools {
		resp.TopTools = append(resp.TopTools, toolCountResp{Tool: t.Tool, Count: t.Count})
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
		AvgLatencyMs:     stats.AvgLatencyMs,
		ToolCalls:        stats.ToolCalls,
	}

	writeJSON(w, http.StatusOK, resp)
}
