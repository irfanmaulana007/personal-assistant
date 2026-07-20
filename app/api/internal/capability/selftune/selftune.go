// Package selftune implements the Self-Tuning skill: it lets the assistant
// review its own recent low-quality conversations and rewrite the instruction
// prompts of its other skills so they do better next time. It backs the
// end-of-day daily routine's self-improvement loop.
package selftune

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

const (
	defaultMaxScore = 4.0
	defaultHours    = 24
	reviewLimit     = 30
	selfKey         = "self_tuning" // never tune the tuner itself
	maxTextChars    = 1200          // truncate long inputs/outputs in the report
	maxToolResChars = 240
)

// routineSources are excluded from the review so the tuner never learns from its
// own scheduled runs.
var routineSources = []string{"start_of_day", "end_of_day"}

// Handler reviews low-quality traces and updates skill prompts.
type Handler struct {
	store store.Store
	log   *slog.Logger
}

// New creates a self-tune handler.
func New(s store.Store, log *slog.Logger) *Handler {
	return &Handler{store: s, log: log.With("component", "selftune")}
}

func (h *Handler) Name() string { return "self_tune" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilitySelfTune
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionSelfTuneReview:
		return h.review(ctx, result)
	case intent.ActionSelfTuneApply:
		return h.apply(ctx, result)
	default:
		return "I can review recent low-quality conversations or update a skill's prompt.", nil
	}
}

// --- review ---

type convoReport struct {
	TraceID   int64    `json:"trace_id"`
	Score     float64  `json:"score"`
	Rationale string   `json:"judge_rationale,omitempty"`
	Skills    []string `json:"skills"`
	Input     string   `json:"user_input"`
	Output    string   `json:"assistant_reply"`
	Tools     []string `json:"tools_called,omitempty"`
}

type reviewReport struct {
	WindowHours   int               `json:"window_hours"`
	MaxScore      float64           `json:"max_score"`
	Count         int               `json:"count"`
	Conversations []convoReport     `json:"conversations"`
	SkillPrompts  map[string]string `json:"current_skill_prompts"`
}

func (h *Handler) review(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	maxScore := parseFloat(result.Entities["max_score"], defaultMaxScore)
	hours := parseInt(result.Entities["hours"], defaultHours)
	if hours <= 0 {
		hours = defaultHours
	}

	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)
	traces, err := h.store.ListLowScoreTracesWithSkills(ctx, userID, from, now, maxScore, routineSources, reviewLimit)
	if err != nil {
		return "", fmt.Errorf("list low-score traces: %w", err)
	}
	if len(traces) == 0 {
		return fmt.Sprintf("No conversations scored at or below %.1f in the last %dh had a skill attached. There is nothing to tune right now.", maxScore, hours), nil
	}

	// Collect the skills that actually appear, then attach their current prompt.
	involved := map[string]bool{}
	convos := make([]convoReport, 0, len(traces))
	for _, t := range traces {
		c := convoReport{
			TraceID: t.ID,
			Skills:  t.Skills,
			Input:   truncate(t.Input, maxTextChars),
			Output:  truncate(t.Output, maxTextChars),
		}
		if t.Score != nil {
			c.Score = t.Score.Overall
			c.Rationale = t.Score.Rationale
		}
		for _, tool := range t.Tools {
			desc := tool.Name
			if r := strings.TrimSpace(tool.Result); r != "" {
				desc += " → " + truncate(r, maxToolResChars)
			}
			c.Tools = append(c.Tools, desc)
		}
		for _, sk := range t.Skills {
			if sk != "" && sk != selfKey {
				involved[sk] = true
			}
		}
		convos = append(convos, c)
	}

	prompts := map[string]string{}
	if len(involved) > 0 {
		skills, err := h.store.ListSkills(ctx)
		if err != nil {
			return "", fmt.Errorf("list skills: %w", err)
		}
		for _, sk := range skills {
			if involved[sk.Key] {
				prompts[sk.Key] = sk.EffectivePrompt()
			}
		}
	}

	rep := reviewReport{
		WindowHours:   hours,
		MaxScore:      maxScore,
		Count:         len(convos),
		Conversations: convos,
		SkillPrompts:  prompts,
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal review: %w", err)
	}
	return string(b), nil
}

// --- apply ---

func (h *Handler) apply(ctx context.Context, result *intent.ParseResult) (string, error) {
	key := strings.TrimSpace(result.Entities["skill"])
	prompt := strings.TrimSpace(result.Entities["prompt"])
	reason := strings.TrimSpace(result.Entities["reason"])
	if key == "" {
		return "Which skill should I update? Pass its key.", nil
	}
	if prompt == "" {
		return "I need the full new prompt text to save for that skill.", nil
	}
	if key == selfKey {
		return "I won't tune the self_tuning skill itself.", nil
	}

	skills, err := h.store.ListSkills(ctx)
	if err != nil {
		return "", fmt.Errorf("list skills: %w", err)
	}
	var found *store.Skill
	valid := make([]string, 0, len(skills))
	for i := range skills {
		valid = append(valid, skills[i].Key)
		if skills[i].Key == key {
			found = &skills[i]
		}
	}
	if found == nil {
		return fmt.Sprintf("No skill with key %q. Valid keys: %s.", key, strings.Join(valid, ", ")), nil
	}

	if err := h.store.UpdateSkillTunedPrompt(ctx, key, prompt); err != nil {
		return "", fmt.Errorf("update skill prompt: %w", err)
	}

	// Read-after-write: re-read the skill and confirm the new tuned prompt
	// actually persisted before telling the user it was updated.
	saved, err := h.store.GetSkill(ctx, found.ID)
	if err != nil {
		return "", fmt.Errorf("verify skill prompt saved: %w", err)
	}
	if saved == nil || saved.TunedPrompt != prompt {
		return "", fmt.Errorf("verify skill prompt saved: stored prompt does not match after update")
	}
	h.log.Info("skill prompt auto-tuned", "skill", key, "reason", reason, "chars", len(prompt))
	msg := fmt.Sprintf("Updated the %q (%s) skill prompt.", found.Name, key)
	if reason != "" {
		msg += " " + reason
	}
	return msg, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}

func parseFloat(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && v > 0 {
		return v
	}
	return def
}

func parseInt(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}
