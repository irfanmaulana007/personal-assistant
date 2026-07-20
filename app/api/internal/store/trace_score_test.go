//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestTraceScoreRoundTrip(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()

	// Two successful traces + one error trace (never judged).
	idA, _ := s.CreateTrace(ctx, &Trace{Platform: "web", Input: "hi", Output: "hello", Model: "m", Status: "ok"})
	idB, _ := s.CreateTrace(ctx, &Trace{Platform: "web", Input: "q", Output: "a", Model: "m", Status: "ok"})
	_, _ = s.CreateTrace(ctx, &Trace{Platform: "web", Input: "boom", Status: "error", Error: "x"})

	// Before any scoring, both successful traces are unscored.
	unscored, err := s.ListUnscoredTraces(ctx, time.Now().AddDate(0, 0, -1), 100)
	if err != nil {
		t.Fatalf("list unscored: %v", err)
	}
	if len(unscored) != 2 {
		t.Fatalf("expected 2 unscored traces, got %d", len(unscored))
	}

	// Judge trace A with a low score.
	if err := s.SaveTraceScore(ctx, &TraceScore{TraceID: idA, Accuracy: 2, Helpfulness: 2, Safety: 3, Overall: 2.33, Rationale: "off", JudgeModel: "judge"}); err != nil {
		t.Fatalf("save score: %v", err)
	}

	// Now only trace B is unscored.
	unscored, _ = s.ListUnscoredTraces(ctx, time.Now().AddDate(0, 0, -1), 100)
	if len(unscored) != 1 || unscored[0].ID != idB {
		t.Fatalf("expected only trace B unscored, got %+v", unscored)
	}

	// GetTraceScore returns the saved verdict; upsert overwrites it.
	if sc, _ := s.GetTraceScore(ctx, idA); sc == nil || sc.Accuracy != 2 {
		t.Fatalf("get score mismatch: %+v", sc)
	}
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idA, Accuracy: 5, Helpfulness: 5, Safety: 5, Overall: 5, JudgeModel: "judge"})
	if sc, _ := s.GetTraceScore(ctx, idA); sc == nil || sc.Overall != 5 {
		t.Fatalf("upsert did not overwrite: %+v", sc)
	}

	// ListTraces joins the score in; the "low" filter respects the threshold.
	now := time.Now()
	all, _ := s.ListTraces(ctx, TraceFilter{From: now.AddDate(0, 0, -1), To: now.AddDate(0, 0, 1)})
	var scored int
	for _, tr := range all {
		if tr.Score != nil {
			scored++
		}
	}
	if scored != 1 {
		t.Fatalf("expected 1 scored trace in list, got %d", scored)
	}

	// After the upsert trace A is 5.0, so "low" should now match nothing.
	low, _ := s.ListTraces(ctx, TraceFilter{From: now.AddDate(0, 0, -1), To: now.AddDate(0, 0, 1), ScoreStates: []string{"low"}})
	if len(low) != 0 {
		t.Fatalf("expected no low-scoring traces, got %d", len(low))
	}
	// Drop it back below threshold and confirm the filter surfaces it.
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idA, Accuracy: 1, Helpfulness: 2, Safety: 2, Overall: 1.67, JudgeModel: "judge"})
	low, _ = s.ListTraces(ctx, TraceFilter{From: now.AddDate(0, 0, -1), To: now.AddDate(0, 0, 1), ScoreStates: []string{"low"}})
	if len(low) != 1 || low[0].ID != idA {
		t.Fatalf("expected trace A in low filter, got %+v", low)
	}

	unscoredFilter, _ := s.ListTraces(ctx, TraceFilter{From: now.AddDate(0, 0, -1), To: now.AddDate(0, 0, 1), ScoreStates: []string{"unscored"}})
	if len(unscoredFilter) != 1 || unscoredFilter[0].ID != idB {
		t.Fatalf("expected trace B via unscored filter, got %+v", unscoredFilter)
	}
}

// TestListTracesMultiFilters exercises the multi-select filters: platforms are
// OR-ed via IN, and score states are OR-ed into a single union.
func TestListTracesMultiFilters(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()

	idA, _ := s.CreateTrace(ctx, &Trace{Platform: "web", Input: "a", Output: "a", Model: "m", Status: "ok"})      // web, low
	idB, _ := s.CreateTrace(ctx, &Trace{Platform: "whatsapp", Input: "b", Output: "b", Model: "m", Status: "ok"}) // whatsapp, unscored
	idC, _ := s.CreateTrace(ctx, &Trace{Platform: "web", Input: "c", Output: "c", Model: "m", Status: "ok"})      // web, high
	_, _ = s.CreateTrace(ctx, &Trace{Platform: "whatsapp", Input: "boom", Status: "error", Error: "x"})           // never judged

	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idA, Accuracy: 2, Helpfulness: 2, Safety: 3, Overall: 2.0, JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idC, Accuracy: 5, Helpfulness: 5, Safety: 5, Overall: 5.0, JudgeModel: "j"})

	from, to := time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1)
	ids := func(f TraceFilter) map[int64]bool {
		f.From, f.To = from, to
		got, err := s.ListTraces(ctx, f)
		if err != nil {
			t.Fatalf("list traces: %v", err)
		}
		m := map[int64]bool{}
		for _, tr := range got {
			m[tr.ID] = true
		}
		return m
	}

	// Platform IN: each single platform, then the union.
	if got := ids(TraceFilter{Platforms: []string{"web"}}); len(got) != 2 || !got[idA] || !got[idC] {
		t.Fatalf("web platform filter: got %v", got)
	}
	if got := ids(TraceFilter{Platforms: []string{"web", "whatsapp"}}); len(got) != 4 {
		t.Fatalf("both platforms: expected 4, got %d", len(got))
	}

	// Score OR: low ∪ unscored surfaces A (low) and B (unscored) but not C (high).
	if got := ids(TraceFilter{ScoreStates: []string{"low", "unscored"}}); len(got) != 2 || !got[idA] || !got[idB] {
		t.Fatalf("low|unscored filter: got %v", got)
	}
	// scored ∪ unscored covers every judgeable trace (A, B, C) but not the error.
	if got := ids(TraceFilter{ScoreStates: []string{"scored", "unscored"}}); len(got) != 3 || !got[idA] || !got[idB] || !got[idC] {
		t.Fatalf("scored|unscored filter: got %v", got)
	}

	// Platform and score AND together: web ∧ (low ∪ unscored) is just A.
	if got := ids(TraceFilter{Platforms: []string{"web"}, ScoreStates: []string{"low", "unscored"}}); len(got) != 1 || !got[idA] {
		t.Fatalf("web + low|unscored filter: got %v", got)
	}
}
