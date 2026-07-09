package api

import (
	"context"
	"net/http"
	"sort"
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
	LatencyP50Ms     int     `json:"latency_p50_ms"`
	LatencyP95Ms     int     `json:"latency_p95_ms"`
	LatencyP99Ms     int     `json:"latency_p99_ms"`
	ToolCalls        int     `json:"tool_calls"`
	Errors           int     `json:"errors"`
	ActiveUsers      int     `json:"active_users"`
}

type usageUserResp struct {
	UserID           int64   `json:"user_id"`
	Name             string  `json:"name"`
	Email            string  `json:"email"`
	Requests         int     `json:"requests"`
	TotalTokens      int     `json:"total_tokens"`
	Errors           int     `json:"errors"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// validPlatform normalizes the platform query param ("" = all).
func validPlatform(p string) string {
	switch p {
	case "web", "whatsapp":
		return p
	default:
		return ""
	}
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
	Date             string  `json:"date"`
	Requests         int     `json:"requests"`
	Errors           int     `json:"errors"`
	TotalTokens      int     `json:"total_tokens"`
	AvgLatencyMs     int     `json:"avg_latency_ms"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
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
	From       string              `json:"from"`     // inclusive, YYYY-MM-DD
	To         string              `json:"to"`       // inclusive, YYYY-MM-DD
	Platform   string              `json:"platform"` // "", web, whatsapp
	Summary    usageSummaryResp    `json:"summary"`
	ByDay      []usageDayResp      `json:"by_day"`
	ByModel    []usageModelResp    `json:"by_model"`
	ByPlatform []usagePlatformResp `json:"by_platform"`
	TopTools   []toolCountResp     `json:"top_tools"`
	ByHour     []int               `json:"by_hour"`    // 24 buckets, UTC
	ByWeekday  []int               `json:"by_weekday"` // 7 buckets (Sun=0), UTC
	ByUser     []usageUserResp     `json:"by_user"`
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

	platform := validPlatform(r.URL.Query().Get("platform"))

	// `to` is inclusive; query with an exclusive end at the start of the next day.
	toExclusive := to.AddDate(0, 0, 1)

	stats, err := s.store.UsageStatsBetween(r.Context(), from, toExclusive, platform)
	if err != nil {
		s.log.Error("failed to load usage stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage"})
		return
	}

	resp := usageResp{
		From:       from.Format(dateLayout),
		To:         to.Format(dateLayout),
		Platform:   platform,
		ByDay:      make([]usageDayResp, 0, len(stats.ByDay)),
		ByModel:    make([]usageModelResp, 0, len(stats.ByModel)),
		ByPlatform: make([]usagePlatformResp, 0, len(stats.ByPlatform)),
		TopTools:   make([]toolCountResp, 0, len(stats.TopTools)),
	}

	// Per-day cost from per-day, per-model token sums.
	costByDay := map[string]float64{}
	if dayModels, err := s.store.UsageByDayModel(r.Context(), from, toExclusive, platform); err == nil {
		for _, dm := range dayModels {
			if c, known := llm.EstimateCost(dm.Model, dm.PromptTokens, dm.CompletionTokens); known {
				costByDay[dm.Date] += c
			}
		}
	}
	for _, d := range stats.ByDay {
		resp.ByDay = append(resp.ByDay, usageDayResp{
			Date: d.Date, Requests: d.Requests, Errors: d.Errors, TotalTokens: d.TotalTokens,
			AvgLatencyMs: d.AvgLatencyMs, EstimatedCostUSD: costByDay[d.Date],
		})
	}
	for _, p := range stats.ByPlatform {
		resp.ByPlatform = append(resp.ByPlatform, usagePlatformResp{Platform: p.Platform, Requests: p.Requests, TotalTokens: p.TotalTokens})
	}
	for _, t := range stats.TopTools {
		resp.TopTools = append(resp.TopTools, toolCountResp{Tool: t.Tool, Count: t.Count})
	}

	resp.ByHour = stats.ByHour[:]
	resp.ByWeekday = stats.ByWeekday[:]
	resp.ByUser = s.usageByUser(r.Context(), from, toExclusive, platform)

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
		LatencyP50Ms:     stats.LatencyP50,
		LatencyP95Ms:     stats.LatencyP95,
		LatencyP99Ms:     stats.LatencyP99,
		ToolCalls:        stats.ToolCalls,
		Errors:           stats.Errors,
		ActiveUsers:      stats.ActiveUsers,
	}

	writeJSON(w, http.StatusOK, resp)
}

// usageByUser aggregates per-user usage (requests/tokens/errors/cost), sorted by
// requests desc, resolving names via ListUsers. Cost is priced per (user, model).
func (s *Server) usageByUser(ctx context.Context, from, to time.Time, platform string) []usageUserResp {
	rows, err := s.store.UsageByUserModel(ctx, from, to, platform)
	if err != nil {
		s.log.Error("usage by user", "error", err)
		return nil
	}

	type nameEmail struct{ name, email string }
	names := map[int64]nameEmail{}
	if users, err := s.store.ListUsers(ctx); err == nil {
		for _, u := range users {
			names[u.ID] = nameEmail{u.Name, u.Email}
		}
	}

	agg := map[int64]*usageUserResp{}
	order := []int64{}
	for _, r := range rows {
		u := agg[r.UserID]
		if u == nil {
			info := names[r.UserID]
			u = &usageUserResp{UserID: r.UserID, Name: info.name, Email: info.email}
			agg[r.UserID] = u
			order = append(order, r.UserID)
		}
		u.Requests += r.Requests
		u.TotalTokens += r.TotalTokens
		u.Errors += r.Errors
		if c, known := llm.EstimateCost(r.Model, r.PromptTokens, r.CompletionTokens); known {
			u.EstimatedCostUSD += c
		}
	}

	out := make([]usageUserResp, 0, len(order))
	for _, id := range order {
		out = append(out, *agg[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	return out
}
