package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type toolInvocationResp struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
	LatencyMs int    `json:"latency_ms,omitempty"`
}

type llmCallResp struct {
	Step             int      `json:"step"`
	Model            string   `json:"model"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	LatencyMs        int      `json:"latency_ms"`
	FinishReason     string   `json:"finish_reason,omitempty"`
	ToolCalls        []string `json:"tool_calls,omitempty"`
	EstimatedCostUSD float64  `json:"estimated_cost_usd"`
}

type traceResp struct {
	ID               int64                `json:"id"`
	UserID           int64                `json:"user_id"`
	User             string               `json:"user,omitempty"`
	Platform         string               `json:"platform"`
	Input            string               `json:"input"`
	Output           string               `json:"output"`
	Model            string               `json:"model"`
	PromptTokens     int                  `json:"prompt_tokens"`
	CompletionTokens int                  `json:"completion_tokens"`
	TotalTokens      int                  `json:"total_tokens"`
	LatencyMs        int                  `json:"latency_ms"`
	ToolCount        int                  `json:"tool_count"`
	Tools            []toolInvocationResp `json:"tools,omitempty"`
	Steps            []llmCallResp        `json:"steps,omitempty"`
	Skills           []string             `json:"skills,omitempty"`
	Status           string               `json:"status"`
	Error            string               `json:"error,omitempty"`
	EstimatedCostUSD float64              `json:"estimated_cost_usd"`
	CreatedAt        string               `json:"created_at"`
}

func (s *Server) traceToResp(ctx context.Context, t *store.Trace, includeTools bool) traceResp {
	cost, _ := s.pricing.Estimate(t.Model, t.PromptTokens, t.CompletionTokens)
	r := traceResp{
		ID:               t.ID,
		UserID:           t.UserID,
		Platform:         t.Platform,
		Input:            t.Input,
		Output:           t.Output,
		Model:            t.Model,
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		TotalTokens:      t.TotalTokens,
		LatencyMs:        t.LatencyMs,
		ToolCount:        t.ToolCount,
		Skills:           t.Skills,
		Status:           t.Status,
		Error:            t.Error,
		EstimatedCostUSD: cost,
		CreatedAt:        t.CreatedAt.Format(time.RFC3339),
	}
	if includeTools {
		for _, tv := range t.Tools {
			r.Tools = append(r.Tools, toolInvocationResp{Name: tv.Name, Arguments: tv.Arguments, Result: tv.Result, LatencyMs: tv.LatencyMs})
		}
		for _, st := range t.Steps {
			c, _ := s.pricing.Estimate(st.Model, st.PromptTokens, st.CompletionTokens)
			r.Steps = append(r.Steps, llmCallResp{
				Step: st.Step, Model: st.Model, PromptTokens: st.PromptTokens,
				CompletionTokens: st.CompletionTokens, TotalTokens: st.TotalTokens,
				LatencyMs: st.LatencyMs, FinishReason: st.FinishReason, ToolCalls: st.ToolCalls,
				EstimatedCostUSD: c,
			})
		}
		if u, err := s.store.GetUserByID(ctx, t.UserID); err == nil && u != nil {
			r.User = u.Email
		}
	}
	return r
}

type logsResp struct {
	Traces     []traceResp `json:"traces"`
	NextCursor int64       `json:"next_cursor,omitempty"`
}

// handleListLogs returns a page of traces (most recent first). Query params:
// from, to (YYYY-MM-DD, default last 30 days), platform, limit, and cursor (the
// id of the last trace from the previous page; 0/absent = first page).
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
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

	limit := 25
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	var cursor int64
	if v, err := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64); err == nil && v > 0 {
		cursor = v
	}

	// Fetch one extra row to know whether a further page exists.
	traces, err := s.store.ListTraces(r.Context(), store.TraceFilter{
		Platform: validPlatform(r.URL.Query().Get("platform")),
		From:     from,
		To:       to.AddDate(0, 0, 1),
		Limit:    limit + 1,
		Cursor:   cursor,
	})
	if err != nil {
		s.log.Error("list traces", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load logs"})
		return
	}

	out := logsResp{Traces: make([]traceResp, 0, limit)}
	if len(traces) > limit {
		out.NextCursor = traces[limit-1].ID
		traces = traces[:limit]
	}
	for i := range traces {
		out.Traces = append(out.Traces, s.traceToResp(r.Context(), &traces[i], false))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetLog returns a single trace with its tool calls.
func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	t, err := s.store.GetTrace(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load trace"})
		return
	}
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, s.traceToResp(r.Context(), t, true))
}
