// Package activity implements the Sports & Workout Summary skill: logging
// sport/workout sessions and summarizing them over a period.
package activity

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler logs and summarizes the user's sport/workout activities.
type Handler struct {
	store    store.Store
	timezone *time.Location
	log      *slog.Logger
}

// New creates an activity handler.
func New(s store.Store, timezone *time.Location, log *slog.Logger) *Handler {
	return &Handler{store: s, timezone: timezone, log: log.With("component", "activity")}
}

func (h *Handler) Name() string { return "activity" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityActivity
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionActivityLog:
		return h.logActivity(ctx, result)
	case intent.ActionActivitySummarize:
		return h.summarize(ctx, result)
	default:
		return "I can log a workout or summarize your recent activity.", nil
	}
}

func (h *Handler) logActivity(ctx context.Context, result *intent.ParseResult) (string, error) {
	actType := strings.TrimSpace(result.Entities["type"])
	if actType == "" {
		return "What kind of activity was it (e.g. running, football, gym)?", nil
	}
	description := strings.TrimSpace(result.Entities["description"])

	occurredAt := time.Now()
	if when := strings.TrimSpace(result.Entities["when"]); when != "" {
		if t, err := capability.ParseTime(when, h.timezone); err == nil {
			occurredAt = t
		}
	}

	userID := authctx.UserID(ctx)
	a, err := h.store.CreateActivity(ctx, userID, actType, description, occurredAt, "chat")
	if err != nil {
		return "", fmt.Errorf("create activity: %w", err)
	}

	// Read-after-write: confirm the activity actually persisted before telling the
	// user it was logged, and build the confirmation from the re-read record.
	a, err = h.store.GetActivity(ctx, userID, a.ID)
	if err != nil {
		return "", fmt.Errorf("verify activity saved: %w", err)
	}
	if a == nil {
		return "", fmt.Errorf("verify activity saved: activity not found after create")
	}
	return fmt.Sprintf("Logged: *%s* on %s.", a.Type, a.OccurredAt.In(h.timezone).Format("Mon, Jan 2")), nil
}

func (h *Handler) summarize(ctx context.Context, result *intent.ParseResult) (string, error) {
	days := 7
	if d := strings.TrimSpace(result.Entities["days"]); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	activities, err := h.store.ListActivitiesSince(ctx, authctx.UserID(ctx), since)
	if err != nil {
		return "", fmt.Errorf("list activities: %w", err)
	}
	if len(activities) == 0 {
		return fmt.Sprintf("No activities logged in the last %d days. Tell me when you work out and I'll keep track!", days), nil
	}

	counts := map[string]int{}
	for _, a := range activities {
		counts[strings.ToLower(a.Type)]++
	}
	type kv struct {
		k string
		n int
	}
	var ranked []kv
	for k, n := range counts {
		ranked = append(ranked, kv{k, n})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].n > ranked[j].n })

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Activity — last %d days*\n%d session(s) total\n\nBy type:", days, len(activities)))
	for _, r := range ranked {
		sb.WriteString(fmt.Sprintf("\n• %s: %d", r.k, r.n))
	}
	sb.WriteString("\n\nRecent:")
	for i, a := range activities {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("\n• %s — %s", a.OccurredAt.In(h.timezone).Format("Jan 2"), a.Type)
		if a.Description != "" {
			line += ": " + a.Description
		}
		sb.WriteString(line)
	}
	return sb.String(), nil
}
