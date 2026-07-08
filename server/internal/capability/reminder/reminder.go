package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"
)

// Handler handles reminder-related commands and runs the background scheduler.
type Handler struct {
	store         store.Store
	timezone      *time.Location
	checkInterval time.Duration
	sendFunc      transport.SendFunc
	ownerJID      string
	log           *slog.Logger
}

// New creates a new reminder handler.
func New(s store.Store, timezone *time.Location, checkInterval time.Duration, ownerJID string, log *slog.Logger) *Handler {
	return &Handler{
		store:         s,
		timezone:      timezone,
		checkInterval: checkInterval,
		ownerJID:      ownerJID,
		log:           log.With("component", "reminder"),
	}
}

// SetSendFunc sets the function used to send reminder notifications.
func (h *Handler) SetSendFunc(fn transport.SendFunc) {
	h.sendFunc = fn
}

func (h *Handler) Name() string { return "reminder" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityReminder
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionReminderSet:
		return h.set(ctx, result)
	case intent.ActionReminderList:
		return h.list(ctx)
	case intent.ActionReminderCancel:
		return h.cancel(ctx, result)
	default:
		return "I can set, list, or cancel reminders. Try: _remind me to call mom at 5pm_", nil
	}
}

func (h *Handler) set(ctx context.Context, result *intent.ParseResult) (string, error) {
	message := result.Entities["message"]
	if message == "" {
		return "What should I remind you about? Example: _remind me to call mom at 5pm_", nil
	}

	timeStr := result.Entities["time"]
	if timeStr == "" {
		return fmt.Sprintf("When should I remind you to %s? Example: _in 30 minutes_, _at 5pm_, _tomorrow at 9am_", message), nil
	}

	remindAt, err := capability.ParseTime(timeStr, h.timezone)
	if err != nil {
		return fmt.Sprintf("I couldn't understand %q as a time. Try: _in 30 minutes_, _at 5pm_, _tomorrow at 9am_", timeStr), nil
	}

	reminder, err := h.store.CreateReminder(ctx, authctx.UserID(ctx), message, remindAt)
	if err != nil {
		return "", fmt.Errorf("create reminder: %w", err)
	}

	return fmt.Sprintf("Reminder set (#%d): _%s_\nI'll remind you %s",
		reminder.ID,
		message,
		remindAt.In(h.timezone).Format("Mon, Jan 2 at 3:04 PM"),
	), nil
}

func (h *Handler) list(ctx context.Context) (string, error) {
	reminders, err := h.store.ListReminders(ctx, authctx.UserID(ctx), true)
	if err != nil {
		return "", fmt.Errorf("list reminders: %w", err)
	}

	if len(reminders) == 0 {
		return "No active reminders.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Active Reminders* (%d):\n\n", len(reminders)))

	for _, r := range reminders {
		sb.WriteString(fmt.Sprintf("#%d — _%s_\n", r.ID, r.Message))
		sb.WriteString(fmt.Sprintf("   Due: %s\n\n", r.RemindAt.In(h.timezone).Format("Mon, Jan 2 at 3:04 PM")))
	}

	sb.WriteString("Cancel with: _cancel reminder N_")
	return sb.String(), nil
}

func (h *Handler) cancel(ctx context.Context, result *intent.ParseResult) (string, error) {
	idStr := result.Entities["id"]
	if idStr == "" {
		return "Which reminder should I cancel? Example: _cancel reminder 1_", nil
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "Please provide a valid reminder number.", nil
	}

	if err := h.store.CancelReminder(ctx, authctx.UserID(ctx), id); err != nil {
		return "", fmt.Errorf("cancel reminder: %w", err)
	}

	return fmt.Sprintf("Reminder #%d cancelled.", id), nil
}

// StartScheduler starts the background reminder checker.
func (h *Handler) StartScheduler(ctx context.Context) {
	h.log.Info("reminder scheduler started", "interval", h.checkInterval)

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.log.Info("reminder scheduler stopped")
			return
		case <-ticker.C:
			h.checkDueReminders(ctx)
		}
	}
}

func (h *Handler) checkDueReminders(ctx context.Context) {
	// Reminders are per-user, but WhatsApp delivery only reaches the owner
	// (the first admin, mapped to the configured owner JID). Other users' due
	// reminders remain listable in chat but are not pushed.
	owner, err := h.store.FirstAdmin(ctx)
	if err != nil {
		h.log.Error("failed to resolve owner for reminders", "error", err)
		return
	}
	if owner == nil {
		return // no admin yet (setup not completed)
	}

	reminders, err := h.store.GetDueReminders(ctx, owner.ID)
	if err != nil {
		h.log.Error("failed to get due reminders", "error", err)
		return
	}

	for _, r := range reminders {
		msg := fmt.Sprintf("⏰ *Reminder*: %s", r.Message)

		if h.sendFunc != nil {
			if err := h.sendFunc(ctx, h.ownerJID, msg); err != nil {
				h.log.Error("failed to send reminder", "id", r.ID, "error", err)
				continue
			}
		}

		if err := h.store.MarkReminderNotified(ctx, r.ID); err != nil {
			h.log.Error("failed to mark reminder notified", "id", r.ID, "error", err)
		}

		h.log.Info("reminder sent", "id", r.ID, "message", r.Message)
	}
}
