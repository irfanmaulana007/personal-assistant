package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// This file holds row-scanning and serialization helpers shared across the
// storage backends. They are dialect-agnostic (plain database/sql scanning plus
// the comma-joined encoding used for reminder list columns) and are consumed by
// the PostgreSQL backend in postgres_*.go. They previously lived alongside the
// (now removed) SQLite backend.

// rowScanner is the subset of *sql.Row / *sql.Rows needed to scan a single row.
type rowScanner interface {
	Scan(dest ...any) error
}

// --- Reminders ---

const reminderCols = `id, user_id, title, message, repeat_mode, times, weekdays, day_of_month, once_date, event_at, offsets, enabled, last_fired_at, calendar_conn, calendar_event_ids, calendar_hash, remind_at, created_at, notified, cancelled`

func scanReminder(row rowScanner) (*Reminder, error) {
	var r Reminder
	var times, weekdays, offsets, calEventIDs string
	var lastFired sql.NullTime
	if err := row.Scan(
		&r.ID, &r.UserID, &r.Title, &r.Message, &r.RepeatMode, &times, &weekdays,
		&r.DayOfMonth, &r.OnceDate, &r.EventAt, &offsets, &r.Enabled, &lastFired,
		&r.CalendarConn, &calEventIDs, &r.CalendarHash,
		&r.RemindAt, &r.CreatedAt, &r.Notified, &r.Cancelled,
	); err != nil {
		return nil, err
	}
	r.Times = splitTimes(times)
	r.Weekdays = splitInts(weekdays)
	r.Offsets = splitInts(offsets)
	r.CalendarEventIDs = splitTimes(calEventIDs) // comma-split, non-empty
	if lastFired.Valid {
		t := lastFired.Time.UTC()
		r.LastFiredAt = &t
	}
	return &r, nil
}

func scanReminders(rows *sql.Rows) ([]Reminder, error) {
	var reminders []Reminder
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminders = append(reminders, *r)
	}
	return reminders, rows.Err()
}

func joinTimes(times []string) string { return strings.Join(times, ",") }

func splitTimes(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func splitInts(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}

// --- Notes ---

func scanNotes(rows *sql.Rows) ([]Note, error) {
	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// --- Bucket list ---

const bucketItemCols = "id, title, description, note, category, resolution_year, done, created_at, done_at"

// --- Memories ---

func scanMemories(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Memory, error) {
	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.Kind, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- Persona ---

// defaultPersona is used when a user has no saved persona.
func defaultPersona() UserPersona {
	return UserPersona{Tone: "balanced", Emoji: "occasional", Length: "balanced", Personality: "balanced"}
}
