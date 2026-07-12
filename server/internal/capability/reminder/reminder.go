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
	"github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
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

// reconcileInterval bounds how often the calendar mirror is reconciled.
const reconcileInterval = 5 * time.Minute

// CalendarSyncer reads the owner's calendar events (for the recap) and mirrors
// reminders to Google Calendar (for the reconciler).
type CalendarSyncer interface {
	ListEvents(ctx context.Context, userID int64, from, to time.Time) []calendar.Event
	PrimaryConnection(ctx context.Context, userID int64) (string, bool)
	CreateEvent(ctx context.Context, userID int64, connID string, ev calendar.Event, rrule string) (string, error)
	DeleteEvent(ctx context.Context, userID int64, connID, eventID string) error
}

// Handler handles reminder-related commands and runs the background scheduler.
type Handler struct {
	store         store.Store
	settings      *settings.Service
	calendar      CalendarSyncer
	timezone      *time.Location
	checkInterval time.Duration
	sendFunc      transport.SendFunc
	ownerJID      string
	log           *slog.Logger

	lastReconcile time.Time
}

// New creates a new reminder handler. cal may be nil (no calendar mirror/recap).
func New(s store.Store, settingsSvc *settings.Service, cal CalendarSyncer, timezone *time.Location, checkInterval time.Duration, ownerJID string, log *slog.Logger) *Handler {
	return &Handler{
		store:         s,
		settings:      settingsSvc,
		calendar:      cal,
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
	case intent.ActionReminderSchedule:
		return h.schedule(ctx, result)
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

	// Store as a proper one-off reminder (a dated "once" with a time) so it shows
	// up alongside reminders created in the app, not just the legacy chat list.
	local := remindAt.In(h.timezone)
	in := store.ReminderInput{
		Title:      message,
		RepeatMode: "once",
		OnceDate:   local.Format("2006-01-02"),
		Times:      []string{local.Format("15:04")},
		Enabled:    true,
	}
	reminder, err := h.store.CreateReminder(ctx, authctx.UserID(ctx), in)
	if err != nil {
		return "", fmt.Errorf("create reminder: %w", err)
	}

	return fmt.Sprintf("Reminder set (#%d): _%s_\nI'll remind you %s",
		reminder.ID,
		message,
		local.Format("Mon, Jan 2 at 3:04 PM"),
	), nil
}

// schedule creates a recurring or dated reminder from structured arguments.
func (h *Handler) schedule(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := strings.TrimSpace(firstNonEmpty(result.Entities["title"], result.Entities["message"]))
	if title == "" {
		return "What should I remind you about?", nil
	}

	repeat := strings.ToLower(strings.TrimSpace(result.Entities["repeat"]))
	switch repeat {
	case "once", "daily", "weekly", "monthly":
	default:
		return "How often should it repeat — once, daily, weekly, or monthly?", nil
	}

	// When no time is given, fall back to the user's configured default time.
	times := parseTimesCSV(result.Entities["times"])
	if len(times) == 0 {
		times = []string{h.defaultTime(ctx)}
	}

	in := store.ReminderInput{Title: title, RepeatMode: repeat, Times: times, Enabled: true}

	switch repeat {
	case "weekly":
		days := parseWeekdaysCSV(result.Entities["weekdays"])
		if len(days) == 0 {
			return "Which days of the week? For example, _Monday and Wednesday_.", nil
		}
		in.Weekdays = days
	case "monthly":
		dom, _ := strconv.Atoi(strings.TrimSpace(result.Entities["day_of_month"]))
		if dom < 1 || dom > 31 {
			return "Which day of the month (1-31)?", nil
		}
		in.DayOfMonth = dom
	case "once":
		date := strings.TrimSpace(result.Entities["date"])
		if _, err := time.ParseInLocation("2006-01-02", date, h.timezone); err != nil {
			return "What date should I remind you (e.g. 2026-08-05)?", nil
		}
		in.OnceDate = date
	}

	reminder, err := h.store.CreateReminder(ctx, authctx.UserID(ctx), in)
	if err != nil {
		return "", fmt.Errorf("create reminder: %w", err)
	}
	return fmt.Sprintf("Reminder scheduled (#%d): _%s_\n%s", reminder.ID, title, describeSchedule(*reminder, h.timezone)), nil
}

// defaultTime returns the user's configured default reminder time (HH:MM),
// falling back to a fixed default when settings are unavailable.
func (h *Handler) defaultTime(ctx context.Context) string {
	if h.settings != nil {
		if hh, mm, ok := parseHM(h.settings.ReminderDefaultTime(ctx)); ok {
			return fmt.Sprintf("%02d:%02d", hh, mm)
		}
	}
	return settings.DefaultReminderTime
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// parseTimesCSV parses a comma-separated list of HH:MM times, normalizing and
// sorting them; invalid entries are skipped.
func parseTimesCSV(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		hh, mm, ok := parseHM(strings.TrimSpace(part))
		if !ok {
			continue
		}
		hm := fmt.Sprintf("%02d:%02d", hh, mm)
		if !seen[hm] {
			seen[hm] = true
			out = append(out, hm)
		}
	}
	sort.Strings(out)
	return out
}

var weekdayAliases = map[string]int{
	"sun": 0, "sunday": 0, "min": 0, "minggu": 0,
	"mon": 1, "monday": 1, "sen": 1, "senin": 1,
	"tue": 2, "tuesday": 2, "sel": 2, "selasa": 2,
	"wed": 3, "wednesday": 3, "rab": 3, "rabu": 3,
	"thu": 4, "thursday": 4, "kam": 4, "kamis": 4,
	"fri": 5, "friday": 5, "jum": 5, "jumat": 5,
	"sat": 6, "saturday": 6, "sab": 6, "sabtu": 6,
}

// parseWeekdaysCSV parses comma-separated weekday names/numbers into 0-6 ints.
func parseWeekdaysCSV(s string) []int {
	var out []int
	seen := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		d := -1
		if n, err := strconv.Atoi(p); err == nil && n >= 0 && n <= 6 {
			d = n
		} else if v, ok := weekdayAliases[p]; ok {
			d = v
		}
		if d >= 0 && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out
}

func (h *Handler) list(ctx context.Context) (string, error) {
	reminders, err := h.store.ListReminders(ctx, authctx.UserID(ctx), true)
	if err != nil {
		return "", fmt.Errorf("list reminders: %w", err)
	}

	if len(reminders) == 0 {
		return "Your schedule is empty — there are no active reminders.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Your schedule* (%d reminders):\n\n", len(reminders)))

	for _, r := range reminders {
		sb.WriteString(fmt.Sprintf("#%d — _%s_\n", r.ID, reminderBody(r)))
		sb.WriteString(fmt.Sprintf("   %s\n\n", describeSchedule(r, h.timezone)))
	}

	sb.WriteString("Cancel with: _cancel reminder N_")
	return sb.String(), nil
}

var weekdayNames = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// describeSchedule renders a reminder's recurrence in human form for the schedule
// listing (e.g. "Every day at 08:00, 20:00", "Weekly on Mon, Wed at 09:00").
func describeSchedule(r store.Reminder, tz *time.Location) string {
	// When a reminder tracks an actual event, the schedule shows the EVENT time —
	// the notification itself may fire earlier (e.g. an hour before).
	if r.EventAt != "" {
		if event, err := time.ParseInLocation("2006-01-02T15:04", r.EventAt, tz); err == nil {
			return "Event on " + event.Format("Mon, Jan 2 at 3:04 PM")
		}
	}
	times := strings.Join(r.Times, ", ")
	switch r.RepeatMode {
	case "daily":
		return "Every day at " + times
	case "weekly":
		days := make([]string, 0, len(r.Weekdays))
		for _, d := range r.Weekdays {
			if d >= 0 && d <= 6 {
				days = append(days, weekdayNames[d])
			}
		}
		return fmt.Sprintf("Weekly on %s at %s", strings.Join(days, ", "), times)
	case "monthly":
		return fmt.Sprintf("Monthly on day %d at %s", r.DayOfMonth, times)
	default: // once (and any event reminder whose event time failed to parse)
		if len(r.Times) == 0 { // legacy one-shot
			return "On " + r.RemindAt.In(tz).Format("Mon, Jan 2 at 3:04 PM")
		}
		if day, err := time.ParseInLocation("2006-01-02", r.OnceDate, tz); err == nil {
			return fmt.Sprintf("Once on %s at %s", day.Format("Mon, Jan 2"), times)
		}
		return "Once at " + times
	}
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
		switch {
		case r.RepeatMode == "specific":
			h.fireSpecific(ctx, r, now) // event + offset-derived reminder points
		case len(r.Times) == 0:
			h.fireLegacy(ctx, r) // one-shot chat reminder (remind_at based)
		default:
			h.fireReminder(ctx, r, now)
		}
	}

	// Periodically reconcile the Google Calendar mirror (create/update/delete).
	if h.calendar != nil && time.Since(h.lastReconcile) >= reconcileInterval {
		h.lastReconcile = time.Now()
		h.reconcileCalendar(ctx, owner.ID)
	}
}

// reconcileCalendar brings the owner's Google Calendar mirror in line with the
// reminders table: creates events for new/changed reminders, deletes them for
// disabled/removed ones. Everything is best-effort (fail-soft).
func (h *Handler) reconcileCalendar(ctx context.Context, ownerID int64) {
	reminders, err := h.store.ListAllForOwner(ctx, ownerID)
	if err != nil {
		h.log.Error("reconcile: list reminders", "error", err)
		return
	}
	connID, hasCal := h.calendar.PrimaryConnection(ctx, ownerID)
	for _, r := range reminders {
		h.reconcileOne(ctx, ownerID, r, connID, hasCal)
	}
}

func (h *Handler) reconcileOne(ctx context.Context, userID int64, r store.Reminder, connID string, hasCal bool) {
	// Removed reminder: clean up its events, then drop the row.
	if r.Cancelled {
		h.deleteMirror(ctx, userID, r)
		if err := h.store.HardDeleteReminder(ctx, r.ID); err != nil {
			h.log.Error("reconcile: hard delete", "id", r.ID, "error", err)
		}
		return
	}
	// Disabled, or no calendar connected: ensure nothing is mirrored.
	if !r.Enabled || !hasCal {
		if len(r.CalendarEventIDs) > 0 {
			h.deleteMirror(ctx, userID, r)
			_ = h.store.ClearReminderCalendar(ctx, r.ID)
		}
		return
	}
	// Enabled + connected: (re)create the mirror when missing or changed.
	want := calendarHash(r)
	if len(r.CalendarEventIDs) > 0 && r.CalendarHash == want && r.CalendarConn == connID {
		return // already in sync
	}
	if len(r.CalendarEventIDs) > 0 {
		h.deleteMirror(ctx, userID, r)
	}
	ids := h.createMirror(ctx, userID, connID, r)
	if len(ids) > 0 {
		if err := h.store.SetReminderCalendar(ctx, r.ID, connID, ids, want); err != nil {
			h.log.Error("reconcile: save refs", "id", r.ID, "error", err)
		}
	} else {
		_ = h.store.ClearReminderCalendar(ctx, r.ID) // retry next cycle
	}
}

func (h *Handler) deleteMirror(ctx context.Context, userID int64, r store.Reminder) {
	for _, eid := range r.CalendarEventIDs {
		if err := h.calendar.DeleteEvent(ctx, userID, r.CalendarConn, eid); err != nil {
			h.log.Warn("reconcile: delete event", "id", eid, "error", err)
		}
	}
}

func (h *Handler) createMirror(ctx context.Context, userID int64, connID string, r store.Reminder) []string {
	var ids []string
	for _, me := range reminderEvents(r, h.timezone) {
		id, err := h.calendar.CreateEvent(ctx, userID, connID, me.ev, me.rrule)
		if err != nil {
			h.log.Warn("reconcile: create event", "reminder", r.ID, "error", err)
			continue
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
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

// fireSpecific evaluates an event reminder whose fire points are derived from
// the event time minus each configured offset. It fires at most one point per
// tick (the most-recent past-due one) and disables itself after the final point.
func (h *Handler) fireSpecific(ctx context.Context, r store.Reminder, now time.Time) {
	event, err := time.ParseInLocation("2006-01-02T15:04", r.EventAt, h.timezone)
	if err != nil || len(r.Offsets) == 0 {
		return
	}

	// Points = event - offset. Track the most-recent past-due point and the
	// latest point overall (which, once fired, means the reminder is done).
	var mostRecent, latest time.Time
	foundDue, foundLatest := false, false
	for _, off := range r.Offsets {
		p := event.Add(-time.Duration(off) * time.Minute)
		if !foundLatest || p.After(latest) {
			latest, foundLatest = p, true
		}
		if p.After(now) {
			continue
		}
		if !foundDue || p.After(mostRecent) {
			mostRecent, foundDue = p, true
		}
	}
	if !foundDue {
		return
	}
	if r.LastFiredAt != nil && !mostRecent.After(*r.LastFiredAt) {
		return
	}
	disable := !mostRecent.Before(latest) // fired the final point

	if now.Sub(mostRecent) > graceWindow {
		if err := h.store.MarkReminderFired(ctx, r.ID, mostRecent, disable); err != nil {
			h.log.Error("failed to advance stale event reminder", "id", r.ID, "error", err)
		}
		h.log.Info("skipped stale event reminder point", "id", r.ID, "point", mostRecent)
		return
	}

	msg := formatEventReminder(reminderBody(r), event, event.Sub(mostRecent), h.timezone)
	if h.sendFunc != nil {
		if err := h.sendFunc(ctx, h.ownerJID, msg); err != nil {
			h.log.Error("failed to send reminder", "id", r.ID, "error", err)
			return
		}
	}
	if err := h.store.MarkReminderFired(ctx, r.ID, mostRecent, disable); err != nil {
		h.log.Error("failed to mark reminder fired", "id", r.ID, "error", err)
	}
	h.log.Info("event reminder sent", "id", r.ID, "point", mostRecent)
}

// formatEventReminder builds the notification body for an event reminder,
// naming the event time and how far away it is.
func formatEventReminder(title string, event time.Time, lead time.Duration, tz *time.Location) string {
	when := event.In(tz).Format("Mon, Jan 2 at 3:04 PM")
	if lead <= time.Minute {
		return fmt.Sprintf("⏰ *Reminder*: %s\n_Happening now — %s_", title, when)
	}
	return fmt.Sprintf("⏰ *Reminder*: %s\n_In %s — %s_", title, humanizeLead(lead), when)
}

// humanizeLead renders a positive duration as a short human string (e.g. "1 day",
// "2 hours", "30 minutes").
func humanizeLead(d time.Duration) string {
	mins := int(d.Round(time.Minute) / time.Minute)
	switch {
	case mins%1440 == 0 && mins >= 1440:
		return plural(mins/1440, "day")
	case mins%60 == 0 && mins >= 60:
		return plural(mins/60, "hour")
	default:
		return plural(mins, "minute")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
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

// mirrorEvent is one calendar event to create for a reminder (one per time).
type mirrorEvent struct {
	ev    calendar.Event
	rrule string
}

var icalWeekdays = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// reminderEvents maps a reminder to the calendar events that mirror it: one per
// time, with a recurrence rule for daily/weekly/monthly. Specific/legacy
// reminders (no times) produce nothing.
func reminderEvents(r store.Reminder, tz *time.Location) []mirrorEvent {
	now := time.Now().In(tz)
	var out []mirrorEvent
	for _, hm := range r.Times {
		hh, mm, ok := parseHM(hm)
		if !ok {
			continue
		}
		var start time.Time
		var rrule string
		switch r.RepeatMode {
		case "daily":
			rrule = "RRULE:FREQ=DAILY"
		case "weekly":
			days := make([]string, 0, len(r.Weekdays))
			for _, d := range r.Weekdays {
				if d >= 0 && d <= 6 {
					days = append(days, icalWeekdays[d])
				}
			}
			if len(days) == 0 {
				continue
			}
			rrule = "RRULE:FREQ=WEEKLY;BYDAY=" + strings.Join(days, ",")
		case "monthly":
			if r.DayOfMonth < 1 || r.DayOfMonth > 31 {
				continue
			}
			rrule = fmt.Sprintf("RRULE:FREQ=MONTHLY;BYMONTHDAY=%d", r.DayOfMonth)
		default: // once
			day, err := time.ParseInLocation("2006-01-02", r.OnceDate, tz)
			if err != nil {
				continue
			}
			start = time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, tz)
		}
		// For recurring reminders anchor DTSTART to the next real occurrence (not
		// "today"), so the event lands on the correct day in Google Calendar and
		// the reconciler's dedup window — centered on the start — matches the
		// existing recurring instance instead of re-creating it every cycle.
		if rrule != "" {
			var ok bool
			if start, ok = nextRecurrenceStart(r, hh, mm, now, tz); !ok {
				continue
			}
		}
		out = append(out, mirrorEvent{
			ev:    calendar.Event{Title: reminderBody(r), Start: start, End: start.Add(time.Hour)},
			rrule: rrule,
		})
	}
	return out
}

// nextRecurrenceStart returns the start instant of the next occurrence of a
// recurring reminder at or after today, at the given time-of-day. It reuses
// dayQualifies so the calendar anchor matches the reminder's own firing rule
// (including monthly day-of-month clamping). ok is false if no day qualifies
// within a year (shouldn't happen for valid daily/weekly/monthly reminders).
func nextRecurrenceStart(r store.Reminder, hh, mm int, now time.Time, tz *time.Location) (time.Time, bool) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
	for i := 0; i < 366; i++ {
		day := today.AddDate(0, 0, i)
		if dayQualifies(r, day) {
			return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, tz), true
		}
	}
	return time.Time{}, false
}

// calendarHash fingerprints the fields that define a reminder's calendar mirror,
// so the reconciler can detect edits and re-sync.
func calendarHash(r store.Reminder) string {
	return strings.Join([]string{
		reminderBody(r), r.RepeatMode,
		strings.Join(r.Times, ","), joinIntSlice(r.Weekdays),
		strconv.Itoa(r.DayOfMonth), r.OnceDate,
	}, "|")
}

func joinIntSlice(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}
