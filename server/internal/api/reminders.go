package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type reminderResp struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	RepeatMode string   `json:"repeat_mode"`
	Times      []string `json:"times"`
	Weekdays   []int    `json:"weekdays"`
	DayOfMonth int      `json:"day_of_month"`
	OnceDate   string   `json:"once_date"`
	EventAt    string   `json:"event_at"`
	Offsets    []int    `json:"offsets"`
	Enabled    bool     `json:"enabled"`
}

type reminderReq struct {
	Title      string   `json:"title"`
	RepeatMode string   `json:"repeat_mode"`
	Times      []string `json:"times"`
	Weekdays   []int    `json:"weekdays"`
	DayOfMonth int      `json:"day_of_month"`
	OnceDate   string   `json:"once_date"`
	EventAt    string   `json:"event_at"`
	Offsets    []int    `json:"offsets"`
	Enabled    bool     `json:"enabled"`
}

func toReminderResp(r store.Reminder) reminderResp {
	return reminderResp{
		ID:         r.ID,
		Title:      r.Title,
		RepeatMode: r.RepeatMode,
		Times:      emptyToSlice(r.Times),
		Weekdays:   emptyToIntSlice(r.Weekdays),
		DayOfMonth: r.DayOfMonth,
		OnceDate:   r.OnceDate,
		EventAt:    r.EventAt,
		Offsets:    emptyToIntSlice(r.Offsets),
		Enabled:    r.Enabled,
	}
}

func emptyToSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func emptyToIntSlice(s []int) []int {
	if s == nil {
		return []int{}
	}
	return s
}

// validateReminder normalizes and validates a create/update payload, returning
// a store.ReminderInput ready to persist. tz is used to reject a once-off in the past.
func validateReminder(req reminderReq, tz *time.Location) (store.ReminderInput, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return store.ReminderInput{}, fmt.Errorf("title is required")
	}

	mode := req.RepeatMode
	switch mode {
	case "once", "daily", "weekly", "monthly", "specific":
	default:
		return store.ReminderInput{}, fmt.Errorf("repeat_mode must be once, daily, weekly, monthly, or specific")
	}

	in := store.ReminderInput{
		Title:      title,
		RepeatMode: mode,
		Enabled:    req.Enabled,
	}

	// The "specific" (event) mode is driven by an event time and lead-time
	// offsets rather than times-of-day.
	if mode == "specific" {
		event, normalized, err := parseEventAt(req.EventAt, tz)
		if err != nil {
			return store.ReminderInput{}, err
		}
		offsets, err := normalizeOffsets(req.Offsets)
		if err != nil {
			return store.ReminderInput{}, err
		}
		// The latest reminder point is event − the smallest offset. If even that
		// is already past, the reminder can never fire.
		latest := event.Add(-time.Duration(offsets[0]) * time.Minute)
		if latest.Before(time.Now().In(tz)) {
			return store.ReminderInput{}, fmt.Errorf("the event and all its reminders are in the past")
		}
		in.EventAt = normalized
		in.Offsets = offsets
		return in, nil
	}

	times, err := normalizeTimes(req.Times)
	if err != nil {
		return store.ReminderInput{}, err
	}
	in.Times = times

	switch mode {
	case "weekly":
		wd, err := normalizeWeekdays(req.Weekdays)
		if err != nil {
			return store.ReminderInput{}, err
		}
		in.Weekdays = wd
	case "monthly":
		if req.DayOfMonth < 1 || req.DayOfMonth > 31 {
			return store.ReminderInput{}, fmt.Errorf("day_of_month must be between 1 and 31")
		}
		in.DayOfMonth = req.DayOfMonth
	case "once":
		day, err := time.ParseInLocation("2006-01-02", req.OnceDate, tz)
		if err != nil {
			return store.ReminderInput{}, fmt.Errorf("once_date must be a valid date (YYYY-MM-DD)")
		}
		// Reject a one-off whose last time is already in the past.
		hh, mm := lastHM(times)
		last := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, tz)
		if last.Before(time.Now().In(tz)) {
			return store.ReminderInput{}, fmt.Errorf("once_date and time are in the past")
		}
		in.OnceDate = req.OnceDate
	}

	// An optional event date/time may accompany any recurring reminder (stored
	// for reference/display; it does not drive firing outside "specific").
	if strings.TrimSpace(req.EventAt) != "" {
		_, normalized, err := parseEventAt(req.EventAt, tz)
		if err != nil {
			return store.ReminderInput{}, err
		}
		in.EventAt = normalized
	}

	return in, nil
}

// parseEventAt parses a local "YYYY-MM-DDTHH:MM" datetime, returning the resolved
// instant and the normalized string.
func parseEventAt(s string, tz *time.Location) (time.Time, string, error) {
	s = strings.TrimSpace(s)
	t, err := time.ParseInLocation("2006-01-02T15:04", s, tz)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("event date & time must be valid (YYYY-MM-DDTHH:MM)")
	}
	return t, t.Format("2006-01-02T15:04"), nil
}

// normalizeOffsets validates lead-time offsets (minutes before the event),
// dedupes, and sorts ascending (smallest = closest to the event).
func normalizeOffsets(offsets []int) ([]int, error) {
	if len(offsets) == 0 {
		return nil, fmt.Errorf("add at least one reminder time before the event")
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(offsets))
	for _, o := range offsets {
		if o < 0 {
			return nil, fmt.Errorf("reminder offsets must not be negative")
		}
		if !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	sort.Ints(out)
	return out, nil
}

// normalizeTimes validates HH:MM entries, zero-pads, dedupes, and sorts ascending.
func normalizeTimes(times []string) ([]string, error) {
	if len(times) == 0 {
		return nil, fmt.Errorf("at least one time is required")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(times))
	for _, t := range times {
		parts := strings.SplitN(strings.TrimSpace(t), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid time %q (use HH:MM)", t)
		}
		hh, err1 := strconv.Atoi(parts[0])
		mm, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
			return nil, fmt.Errorf("invalid time %q (use HH:MM)", t)
		}
		hm := fmt.Sprintf("%02d:%02d", hh, mm)
		if !seen[hm] {
			seen[hm] = true
			out = append(out, hm)
		}
	}
	sort.Strings(out)
	return out, nil
}

func normalizeWeekdays(days []int) ([]int, error) {
	if len(days) == 0 {
		return nil, fmt.Errorf("select at least one weekday")
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(days))
	for _, d := range days {
		if d < 0 || d > 6 {
			return nil, fmt.Errorf("weekdays must be between 0 (Sun) and 6 (Sat)")
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out, nil
}

func lastHM(times []string) (int, int) {
	if len(times) == 0 {
		return 0, 0
	}
	last := times[len(times)-1] // times are sorted ascending
	parts := strings.SplitN(last, ":", 2)
	hh, _ := strconv.Atoi(parts[0])
	mm, _ := strconv.Atoi(parts[1])
	return hh, mm
}

// reminderTimezone resolves the display timezone used for once-off validation.
func (s *Server) reminderTimezone(r *http.Request) *time.Location {
	loc, err := time.LoadLocation(s.readPref(r, prefTimezoneKey, defaultTimezone))
	if err != nil {
		return time.UTC
	}
	return loc
}

// handleListReminders returns the current user's reminders.
func (s *Server) handleListReminders(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	reminders, err := s.store.ListReminders(r.Context(), claims.UserID(), false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load reminders"})
		return
	}
	out := make([]reminderResp, 0, len(reminders))
	for _, rm := range reminders {
		// Hide legacy chat one-shots (empty times, not an event reminder) from
		// the management UI; recurring and specific reminders are shown.
		if len(rm.Times) == 0 && rm.RepeatMode != "specific" {
			continue
		}
		out = append(out, toReminderResp(rm))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateReminder creates a reminder for the current user.
func (s *Server) handleCreateReminder(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req reminderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	in, err := validateReminder(req, s.reminderTimezone(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rm, err := s.store.CreateReminder(r.Context(), claims.UserID(), in)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create reminder"})
		return
	}
	writeJSON(w, http.StatusOK, toReminderResp(*rm))
}

// handleUpdateReminder updates an existing reminder.
func (s *Server) handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req reminderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	in, err := validateReminder(req, s.reminderTimezone(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.UpdateReminder(r.Context(), claims.UserID(), id, in); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update reminder"})
		return
	}
	rm, err := s.store.GetReminder(r.Context(), claims.UserID(), id)
	if err != nil || rm == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "reminder not found"})
		return
	}
	writeJSON(w, http.StatusOK, toReminderResp(*rm))
}

// handleDeleteReminder deletes a reminder.
func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteReminder(r.Context(), claims.UserID(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete reminder"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSetReminderEnabled toggles a reminder's enabled flag without re-running
// full validation, so a reminder can always be paused regardless of its state.
func (s *Server) handleSetReminderEnabled(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.store.SetReminderEnabled(r.Context(), claims.UserID(), id, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update reminder"})
		return
	}
	rm, err := s.store.GetReminder(r.Context(), claims.UserID(), id)
	if err != nil || rm == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "reminder not found"})
		return
	}
	writeJSON(w, http.StatusOK, toReminderResp(*rm))
}

type remindersConfigResp struct {
	Enabled     bool   `json:"enabled"`
	DefaultTime string `json:"default_time"` // local "HH:MM" used when a reminder has no time
}

// handleGetRemindersConfig returns the global reminders settings (any user).
func (s *Server) handleGetRemindersConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, remindersConfigResp{
		Enabled:     s.settings.RemindersEnabled(r.Context()),
		DefaultTime: s.settings.ReminderDefaultTime(r.Context()),
	})
}

// handleSetRemindersConfig sets the global reminders settings (admin only).
func (s *Server) handleSetRemindersConfig(w http.ResponseWriter, r *http.Request) {
	var req remindersConfigResp
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	// The default reminder time is required (a reminder always needs a fire time).
	def, err := normalizeTimes([]string{strings.TrimSpace(req.DefaultTime)})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "default_time must be HH:MM"})
		return
	}
	if err := s.settings.SetRemindersEnabled(r.Context(), req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	if err := s.settings.SetReminderDefaultTime(r.Context(), def[0]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	writeJSON(w, http.StatusOK, remindersConfigResp{Enabled: req.Enabled, DefaultTime: def[0]})
}
