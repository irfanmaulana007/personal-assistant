//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// TestImageUsageAggregation verifies that image-generation usage is persisted on
// the trace (and its tools) and folded into the usage aggregations as its own
// model, kept apart from the LLM's token columns.
func TestImageUsageAggregation(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()

	// One run that used the LLM and generated an image, plus one plain LLM run.
	id, err := s.CreateTrace(ctx, &Trace{
		UserID: 7, Platform: "web", Input: "draw a fox", Output: "here you go", Model: "deepseek-v4-flash",
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		ImageModel: "gpt-image-1-mini", ImagePromptTokens: 20, ImageCompletionTokens: 1000, ImageTotalTokens: 1020,
		Status: "ok",
		Tools: []ToolInvocation{{
			Name: "generate_image", Arguments: `{"prompt":"a fox"}`, Result: "done",
			Model: "gpt-image-1-mini", PromptTokens: 20, CompletionTokens: 1000, TotalTokens: 1020,
		}},
	})
	if err != nil {
		t.Fatalf("create image trace: %v", err)
	}
	_, _ = s.CreateTrace(ctx, &Trace{
		UserID: 7, Platform: "web", Input: "hi", Output: "hello", Model: "deepseek-v4-flash",
		PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Status: "ok",
	})

	// GetTrace round-trips the image aggregate and the per-tool usage.
	got, err := s.GetTrace(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("get trace: %v", err)
	}
	if got.ImageModel != "gpt-image-1-mini" || got.ImageTotalTokens != 1020 {
		t.Fatalf("image aggregate not persisted: %+v", got)
	}
	if len(got.Tools) != 1 || got.Tools[0].Model != "gpt-image-1-mini" || got.Tools[0].TotalTokens != 1020 {
		t.Fatalf("per-tool usage not persisted: %+v", got.Tools)
	}

	from := time.Now().AddDate(0, 0, -1)
	to := time.Now().AddDate(0, 0, 1)

	stats, err := s.UsageStatsBetween(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	// Summary tokens are combined (LLM 150+15 + image 1020).
	if stats.Summary.TotalTokens != 1185 {
		t.Fatalf("summary total tokens = %d, want 1185 (combined)", stats.Summary.TotalTokens)
	}
	// The image model appears as its own by-model row, apart from the LLM.
	var llm, img *UsageModel
	for i := range stats.ByModel {
		switch stats.ByModel[i].Model {
		case "deepseek-v4-flash":
			llm = &stats.ByModel[i]
		case "gpt-image-1-mini":
			img = &stats.ByModel[i]
		}
	}
	if llm == nil || llm.TotalTokens != 165 {
		t.Fatalf("llm by-model row wrong: %+v", llm)
	}
	if img == nil || img.TotalTokens != 1020 || img.PromptTokens != 20 || img.CompletionTokens != 1000 {
		t.Fatalf("image by-model row wrong: %+v", img)
	}

	// Per-user usage includes an image row that does NOT inflate request count.
	users, err := s.UsageByUserModel(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("usage by user/model: %v", err)
	}
	var totalReq, imgRows int
	for _, u := range users {
		if u.UserID != 7 {
			continue
		}
		totalReq += u.Requests
		if u.Model == "gpt-image-1-mini" {
			imgRows++
			if u.Requests != 0 {
				t.Fatalf("image user row should not count requests, got %d", u.Requests)
			}
		}
	}
	if totalReq != 2 {
		t.Fatalf("per-user requests = %d, want 2 (image row must not double-count)", totalReq)
	}
	if imgRows != 1 {
		t.Fatalf("expected 1 image user row, got %d", imgRows)
	}

	// Per-day/model includes an image row for the cost time series.
	days, err := s.UsageByDayModel(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("usage by day/model: %v", err)
	}
	var sawImageDay bool
	for _, d := range days {
		if d.Model == "gpt-image-1-mini" && d.CompletionTokens == 1000 {
			sawImageDay = true
		}
	}
	if !sawImageDay {
		t.Fatalf("expected an image row in by-day/model, got %+v", days)
	}
}
