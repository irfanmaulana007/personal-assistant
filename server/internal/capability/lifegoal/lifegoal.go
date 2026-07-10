// Package lifegoal implements the Life Goals skill: a simple checklist of things
// the user wants to do in life ("Take a swimming course", "Gym membership").
package lifegoal

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler creates, lists, checks off, and deletes the user's life goals.
type Handler struct {
	store store.Store
	log   *slog.Logger
}

// New creates a life-goal handler.
func New(s store.Store, log *slog.Logger) *Handler {
	return &Handler{store: s, log: log.With("component", "lifegoal")}
}

func (h *Handler) Name() string { return "life_goal" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityLifeGoal
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionLifeGoalAdd:
		return h.add(ctx, result)
	case intent.ActionLifeGoalList:
		return h.list(ctx, result)
	case intent.ActionLifeGoalCheck:
		return h.check(ctx, result)
	case intent.ActionLifeGoalDelete:
		return h.remove(ctx, result)
	default:
		return "I can add a life goal, list your goals, check one off, or delete one.", nil
	}
}

func (h *Handler) add(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := strings.TrimSpace(result.Entities["title"])
	if title == "" {
		return "What would you like to add to your life list?", nil
	}
	note := strings.TrimSpace(result.Entities["note"])
	g, err := h.store.CreateLifeGoal(ctx, authctx.UserID(ctx), title, note)
	if err != nil {
		return "", fmt.Errorf("create life goal: %w", err)
	}
	msg := fmt.Sprintf("Added to your life list: *%s* ☐", g.Title)
	if g.Note != "" {
		msg += "\n" + g.Note
	}
	return msg, nil
}

func (h *Handler) list(ctx context.Context, _ *intent.ParseResult) (string, error) {
	goals, err := h.store.ListLifeGoals(ctx, authctx.UserID(ctx))
	if err != nil {
		return "", fmt.Errorf("list life goals: %w", err)
	}
	if len(goals) == 0 {
		return "Your life list is empty. Tell me something you want to do, like \"add gym membership to my life list\".", nil
	}
	done := 0
	for _, g := range goals {
		if g.Done {
			done++
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Your life list* — %d of %d done:\n", done, len(goals)))
	for i, g := range goals {
		box := "☐"
		if g.Done {
			box = "☑"
		}
		sb.WriteString(fmt.Sprintf("\n%d. %s %s", i+1, box, g.Title))
		if g.Note != "" {
			sb.WriteString("\n   " + g.Note)
		}
	}
	return sb.String(), nil
}

func (h *Handler) check(ctx context.Context, result *intent.ParseResult) (string, error) {
	g, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if g == nil {
		return "I couldn't find that on your life list. Try \"list my life goals\" to see them.", nil
	}
	if g.Done {
		return fmt.Sprintf("*%s* is already checked off. 🎉", g.Title), nil
	}
	if err := h.store.SetLifeGoalDone(ctx, authctx.UserID(ctx), g.ID, true); err != nil {
		return "", fmt.Errorf("check life goal: %w", err)
	}
	return fmt.Sprintf("Checked off *%s* ☑ — nice one! 🎉", g.Title), nil
}

func (h *Handler) remove(ctx context.Context, result *intent.ParseResult) (string, error) {
	g, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if g == nil {
		return "I couldn't find that on your life list.", nil
	}
	if err := h.store.DeleteLifeGoal(ctx, authctx.UserID(ctx), g.ID); err != nil {
		return "", fmt.Errorf("delete life goal: %w", err)
	}
	return fmt.Sprintf("Removed *%s* from your life list.", g.Title), nil
}

// find resolves an item reference — either a 1-based position from the last
// listing, a database id, or a case-insensitive title match.
func (h *Handler) find(ctx context.Context, ref string) (*store.LifeGoal, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	goals, err := h.store.ListLifeGoals(ctx, authctx.UserID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list life goals: %w", err)
	}
	// Numeric ref: try list position first, then fall back to a database id.
	if n, err := strconv.Atoi(ref); err == nil {
		if n >= 1 && n <= len(goals) {
			return &goals[n-1], nil
		}
		for i := range goals {
			if goals[i].ID == int64(n) {
				return &goals[i], nil
			}
		}
	}
	// Title match: prefer an exact (case-insensitive) hit, else a substring.
	lower := strings.ToLower(ref)
	for i := range goals {
		if strings.EqualFold(goals[i].Title, ref) {
			return &goals[i], nil
		}
	}
	for i := range goals {
		if strings.Contains(strings.ToLower(goals[i].Title), lower) {
			return &goals[i], nil
		}
	}
	return nil, nil
}
