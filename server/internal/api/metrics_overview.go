package api

import (
	"net/http"
	"strconv"
	"time"
)

type projectOverviewRow struct {
	ProjectID     int64   `json:"project_id"`
	Name          string  `json:"name"`
	MemberCount   int     `json:"member_count"`
	EnabledSkills int     `json:"enabled_skills"`
	Requests      int     `json:"requests"`
	TotalTokens   int     `json:"total_tokens"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
}

type adminOverviewResp struct {
	From     string               `json:"from"`
	To       string               `json:"to"`
	Projects []projectOverviewRow `json:"projects"`
	Summary  usageSummaryResp     `json:"summary"`
}

// handleAdminOverview returns a superadmin cross-project usage & metrics
// overview: a per-project breakdown (members, enabled skills, and usage) plus
// platform-wide usage totals. Query params: from, to (YYYY-MM-DD, UTC; default
// last 30 days) and optional projectId to focus one project.
func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	toExclusive := to.AddDate(0, 0, 1)

	filterID, _ := strconv.ParseInt(r.URL.Query().Get("projectId"), 10, 64)

	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load projects"})
		return
	}

	// Per-project usage (requests/tokens/cost) from traces tagged with project_id,
	// aggregated across models so cost is priced per model.
	type acc struct {
		requests, tokens int
		cost             float64
	}
	usageByID := map[int64]*acc{}
	if rows, err := s.store.UsageByProject(ctx, from, toExclusive); err == nil {
		for _, u := range rows {
			a := usageByID[u.ProjectID]
			if a == nil {
				a = &acc{}
				usageByID[u.ProjectID] = a
			}
			a.requests += u.Requests
			a.tokens += u.TotalTokens
			if c, known := s.pricing.Estimate(u.Model, u.PromptTokens, u.CompletionTokens); known {
				a.cost += c
			}
		}
	} else {
		s.log.Error("usage by project", "error", err)
	}

	rows := make([]projectOverviewRow, 0, len(projects))
	for _, p := range projects {
		if filterID > 0 && p.ID != filterID {
			continue
		}
		members, _ := s.store.ListProjectMembers(ctx, p.ID)
		keys, _ := s.store.EnabledProjectSkillKeys(ctx, p.ID)
		row := projectOverviewRow{
			ProjectID: p.ID, Name: p.Name, MemberCount: len(members), EnabledSkills: len(keys),
		}
		if a := usageByID[p.ID]; a != nil {
			row.Requests = a.requests
			row.TotalTokens = a.tokens
			row.EstimatedCost = a.cost
		}
		rows = append(rows, row)
	}

	stats, err := s.store.UsageStatsBetween(ctx, from, toExclusive, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage"})
		return
	}
	var totalCost float64
	for _, m := range stats.ByModel {
		if c, known := s.pricing.Estimate(m.Model, m.PromptTokens, m.CompletionTokens); known {
			totalCost += c
		}
	}

	writeJSON(w, http.StatusOK, adminOverviewResp{
		From:     from.Format(dateLayout),
		To:       to.Format(dateLayout),
		Projects: rows,
		Summary: usageSummaryResp{
			Requests:         stats.Summary.Requests,
			PromptTokens:     stats.Summary.PromptTokens,
			CompletionTokens: stats.Summary.CompletionTokens,
			TotalTokens:      stats.Summary.TotalTokens,
			EstimatedCostUSD: totalCost,
			AvgLatencyMs:     stats.AvgLatencyMs,
			ToolCalls:        stats.ToolCalls,
			Errors:           stats.Errors,
			ActiveUsers:      stats.ActiveUsers,
		},
	})
}
