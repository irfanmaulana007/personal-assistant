package reminder

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
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
	"github.com/irfanmaulana007/personal-assistant/server/internal/transport"
)

// graceWindow bounds how late a missed slot may fire after a restart/outage.
// Slots older than this are silently skipped (marker advanced, no send) to avoid
// blasting stale reminders on boot. Must be well above the tick interval.
const graceWindow = 15 * time.Minute

// Handler handles reminder-related commands and runs the background scheduler.
type Handler struct {
	store         store.Store
	settings      *settings.Service
	timezone      *time.Location
	checkInterval time.Duration
	sendFunc      transport.SendFunc
	ownerJID      string
	log           *slog.Logger
}

// New creates a new reminder handler.
func New(s store.Store, settingsSvc *settings.Service, timezone *time.Location, checkInterval time.Duration, ownerJID string, log *slog.Logger) *Handler {
	return &Handler{
		store:         s,
		settings:      settingsSvc,
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

	reminder, err := h.store.CreateLegacyReminder(ctx, authctx.UserID(ctx), message, remindAt)
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
	// Global on/off toggle. When disabled, do nothing — importantly, we do not
	// advance any last_fired_at, so re-enabling resumes cleanly.
	if h.settings != nil && !h.settings.RemindersEnabled(ctx) {
		return
	}

	// Reminders are per-user, but WhatsApp delivery only reaches the owner
	// (the first admin, mapped to the configured owner JID). Other users'
	// reminders remain manageable in the UI but are not pushed.
	owner, err := h.store.FirstAdmin(ctx)
	if err != nil {
		h.log.Error("failed to resolve owner for reminders", "error", err)
		return
	}
	if owner == nil {
		return // no admin yet (setup not completed)
	}

	now := time.Now().In(h.timezone)

	reminders, err := h.store.ListEnabledForOwner(ctx, owner.ID)
	if err != nil {
		h.log.Error("failed to list reminders", "error", err)
		return
	}

	for _, r := range reminders {
		if len(r.Times) == 0 {
			h.fireLegacy(ctx, r) // one-shot chat reminder (remind_at based)
			continue
		}
		h.fireReminder(ctx, r, now)
	}
}

// fireLegacy handles the pre-recurrence one-shot reminders (Times empty): fire
// when remind_at has passed, then mark notified.
func (h *Handler) fireLegacy(ctx context.Context, r store.Reminder) {
	if r.Notified || r.RemindAt.After(time.Now().UTC()) {
		return
	}
	msg := fmt.Sprintf("⏰ *Reminder*: %s", reminderBody(r))
	if h.sendFunc != nil {
		if err := h.sendFunc(ctx, h.ownerJID, msg); err != nil {
			h.log.Error("failed to send reminder", "id", r.ID, "error", err)
			return
		}
	}
	if err := h.store.MarkReminderNotified(ctx, r.ID); err != nil {
		h.log.Error("failed to mark reminder notified", "id", r.ID, "error", err)
	}
	h.log.Info("reminder sent", "id", r.ID)
}

// fireReminder evaluates a recurring reminder and sends at most one notification
// per tick (the most-recent past-due slot), advancing last_fired_at so a slot
// never fires twice.
func (h *Handler) fireReminder(ctx context.Context, r store.Reminder, now time.Time) {
	slot, ok := h.mostRecentSlot(r, now)
	if !ok {
		return
	}
	// Monotonic-advance guard: only fire a slot strictly newer than the last.
	if r.LastFiredAt != nil && !slot.After(*r.LastFiredAt) {
		return
	}
	disable := h.isDone(r, slot)

	// Bounded catch-up: skip stale slots but still advance the marker (and
	// disable a spent one-shot) so they neither fire late nor loop forever.
	if now.Sub(slot) > graceWindow {
		if err := h.store.MarkReminderFired(ctx, r.ID, slot, disable); err != nil {
			h.log.Error("failed to advance stale reminder", "id", r.ID, "error", err)
		}
		h.log.Info("skipped stale reminder slot", "id", r.ID, "slot", slot)
		return
	}

	msg := fmt.Sprintf("⏰ *Reminder*: %s", reminderBody(r))
	if h.sendFunc != nil {
		if err := h.sendFunc(ctx, h.ownerJID, msg); err != nil {
			h.log.Error("failed to send reminder", "id", r.ID, "error", err)
			return
		}
	}
	if err := h.store.MarkReminderFired(ctx, r.ID, slot, disable); err != nil {
		h.log.Error("failed to mark reminder fired", "id", r.ID, "error", err)
	}
	h.log.Info("reminder sent", "id", r.ID, "slot", slot)
}

// mostRecentSlot returns the latest scheduled instant (in the app timezone) that
// is at or before now, across the reminder's times over a two-day local
// lookback, honoring the repeat mode. ok is false when nothing is due.
func (h *Handler) mostRecentSlot(r store.Reminder, now time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.timezone)
	for _, day := range []time.Time{today.AddDate(0, 0, -1), today} {
		if !dayQualifies(r, day) {
			continue
		}
		for _, hm := range r.Times {
			hh, mm, ok := parseHM(hm)
			if !ok {
				continue
			}
			slot := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, h.timezone)
			if slot.After(now) {
				continue
			}
			if !found || slot.After(best) {
				best, found = slot, true
			}
		}
	}
	return best, found
}

// isDone reports whether a one-shot reminder has fired its last time for the day
// and should be auto-disabled.
func (h *Handler) isDone(r store.Reminder, slot time.Time) bool {
	if r.RepeatMode != "once" {
		return false
	}
	maxHM := ""
	for _, hm := range r.Times {
		if hm > maxHM {
			maxHM = hm
		}
	}
	hh, mm, ok := parseHM(maxHM)
	if !ok {
		return true
	}
	last := time.Date(slot.Year(), slot.Month(), slot.Day(), hh, mm, 0, 0, h.timezone)
	return !slot.Before(last)
}

// dayQualifies reports whether the reminder's repeat rule matches the local day.
func dayQualifies(r store.Reminder, day time.Time) bool {
	switch r.RepeatMode {
	case "daily":
		return true
	case "weekly":
		wd := int(day.Weekday()) // 0=Sun..6=Sat
		for _, d := range r.Weekdays {
			if d == wd {
				return true
			}
		}
		return false
	case "monthly":
		return day.Day() == effectiveDOM(r.DayOfMonth, day)
	default: // once
		return r.OnceDate == day.Format("2006-01-02")
	}
}

// effectiveDOM clamps a day-of-month to the last day of the given month, so
// e.g. day 31 fires on Feb 28/29 and 30-day months' last day.
func effectiveDOM(dom int, day time.Time) int {
	last := time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, day.Location()).Day()
	if dom > last {
		return last
	}
	return dom
}

func parseHM(hm string) (int, int, bool) {
	parts := strings.SplitN(hm, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

// SortTimes returns the times sorted ascending (helper for callers building input).
func SortTimes(times []string) []string {
	out := append([]string(nil), times...)
	sort.Strings(out)
	return out
}

// reminderBody is the human text sent in a notification: the title, or the
// legacy message when no title is set.
func reminderBody(r store.Reminder) string {
	if r.Title != "" {
		return r.Title
	}
	return r.Message
}
