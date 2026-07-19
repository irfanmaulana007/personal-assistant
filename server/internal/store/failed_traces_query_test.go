//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// TestListFailedTraces covers the auto-triage scan query: it must return traces
// the assistant couldn't handle — either the run errored or the judge scored it
// below LowScoreThreshold — for the given user in the window, with errors sorted
// ahead of low-score replies, excluding the triage routine's own runs and
// populating Skills/Tools for the report.
func TestListFailedTraces(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()
	const uid = int64(7)

	// errored run → should match (and sort first, since it has no score)
	idErr, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "boom", Model: "m", Status: "error", Error: "connect: timeout",
		Skills: []string{"web_search"}, Tools: []ToolInvocation{{Name: "web_search", Result: "err"}}})
	// low score → should match
	idLow, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "q", Output: "bad", Model: "m", Status: "ok",
		Skills: []string{"bucket_list"}})
	// good score → excluded
	idGood, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "g", Output: "great", Model: "m", Status: "ok"})
	// low score but a DIFFERENT user → excluded
	idOther, _ := s.CreateTrace(ctx, &Trace{UserID: 99, Platform: "web", Input: "z", Output: "w", Model: "m", Status: "ok"})
	// errored run from the triage routine itself → excluded via excludeSources
	idRoutine, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "whatsapp", Input: "n", Model: "m", Status: "error", Error: "x", Source: "nightly_triage"})

	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idLow, Accuracy: 1, Helpfulness: 2, Safety: 2, Overall: 1.5, Rationale: "poor", JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idGood, Accuracy: 5, Helpfulness: 5, Safety: 5, Overall: 5.0, JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idOther, Accuracy: 1, Helpfulness: 1, Safety: 1, Overall: 1.0, JudgeModel: "j"})

	now := time.Now()
	got, err := s.ListFailedTraces(ctx, uid, now.Add(-24*time.Hour), now.Add(time.Hour), routineSourcesForTest(), 50)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(got), got)
	}
	// Errors (no score) sort ahead of the low-score reply.
	if got[0].ID != idErr || got[1].ID != idLow {
		t.Fatalf("expected [err=%d, low=%d], got [%d,%d]", idErr, idLow, got[0].ID, got[1].ID)
	}
	if got[0].Status != "error" || got[0].Error == "" {
		t.Fatalf("error trace not populated: %+v", got[0])
	}
	// Detail is populated for the report.
	if len(got[0].Skills) == 0 || got[0].Skills[0] != "web_search" {
		t.Fatalf("skills not populated: %+v", got[0].Skills)
	}
	if len(got[0].Tools) == 0 || got[0].Tools[0].Name != "web_search" {
		t.Fatalf("tools not populated: %+v", got[0].Tools)
	}
	_ = idRoutine
}

func routineSourcesForTest() []string {
	return []string{"nightly_triage", "start_of_day", "end_of_day"}
}
