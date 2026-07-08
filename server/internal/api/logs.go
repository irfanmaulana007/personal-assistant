package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type toolInvocationResp struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

type traceResp struct {
	ID               int64                `json:"id"`
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
	Status           string               `json:"status"`
	Error            string               `json:"error,omitempty"`
	EstimatedCostUSD float64              `json:"estimated_cost_usd"`
	CreatedAt        string               `json:"created_at"`
}

func traceToResp(t *store.Trace, includeTools bool) traceResp {
	cost, _ := llm.EstimateCost(t.Model, t.PromptTokens, t.CompletionTokens)
	r := traceResp{
		ID:               t.ID,
		Platform:         t.Platform,
		Input:            t.Input,
		Output:           t.Output,
		Model:            t.Model,
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		TotalTokens:      t.TotalTokens,
		LatencyMs:        t.LatencyMs,
		ToolCount:        t.ToolCount,
		Status:           t.Status,
		Error:            t.Error,
		EstimatedCostUSD: cost,
		CreatedAt:        t.CreatedAt.Format(time.RFC3339),
	}
	if includeTools {
		for _, tv := range t.Tools {
			r.Tools = append(r.Tools, toolInvocationResp{Name: tv.Name, Arguments: tv.Arguments, Result: tv.Result})
		}
	}
	return r
}

type logsResp struct {
	Traces []traceResp `json:"traces"`
}

// handleListLogs returns a page of traces (most recent first). Query params:
// from, to (YYYY-MM-DD, default last 30 days), platform, limit, offset.
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

	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}

	traces, err := s.store.ListTraces(r.Context(), store.TraceFilter{
		Platform: validPlatform(r.URL.Query().Get("platform")),
		From:     from,
		To:       to.AddDate(0, 0, 1),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		s.log.Error("list traces", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load logs"})
		return
	}

	out := logsResp{Traces: make([]traceResp, 0, len(traces))}
	for i := range traces {
		out.Traces = append(out.Traces, traceToResp(&traces[i], false))
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
	writeJSON(w, http.StatusOK, traceToResp(t, true))
}
