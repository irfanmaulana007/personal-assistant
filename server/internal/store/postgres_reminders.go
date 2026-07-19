package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/jackc/pgx/v5"
)

// --- Reminders ---

// pgReminderCols is the reminders column list in the order pgScanReminder
// expects. It mirrors the SQLite reminderCols ordering exactly.
const pgReminderCols = `id, user_id, title, message, repeat_mode, times, weekdays, day_of_month, once_date, event_at, offsets, enabled, last_fired_at, calendar_conn, calendar_event_ids, calendar_hash, remind_at, created_at, notified, cancelled`

func (s *PostgresStore) CreateReminder(ctx context.Context, userID int64, in ReminderInput) (*Reminder, error) {
	in.Title = s.enTitle(ctx, in.Title)
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO reminders (user_id, project_id, title, message, repeat_mode, times, weekdays, day_of_month, once_date, event_at, offsets, enabled, remind_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
		userID, authctx.ProjectID(ctx), in.Title, in.Title, in.RepeatMode,
		joinTimes(in.Times), joinInts(in.Weekdays), in.DayOfMonth, in.OnceDate, in.EventAt, joinInts(in.Offsets), in.Enabled,
		now, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	return s.GetReminder(ctx, userID, id)
}

// CreateLegacyReminder inserts a one-shot reminder for the natural-language chat
// path (no wall-clock recurrence semantics). Times stays empty so the scheduler
// uses the legacy remind_at branch.
func (s *PostgresStore) CreateLegacyReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error) {
	message = s.enTitle(ctx, message)
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO reminders (user_id, project_id, title, message, repeat_mode, remind_at, created_at) VALUES ($1, $2, $3, $4, 'once', $5, $6) RETURNING id`,
		userID, authctx.ProjectID(ctx), message, message, remindAt.UTC(), now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	return &Reminder{ID: id, UserID: userID, Title: message, Message: message, RemindAt: remindAt, CreatedAt: now, Enabled: true}, nil
}

func (s *PostgresStore) GetReminder(ctx context.Context, userID, id int64) (*Reminder, error) {
	return pgScanReminder(s.pool.QueryRow(ctx,
		`SELECT `+pgReminderCols+` FROM reminders WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3) AND cancelled = false`, id, userID, authctx.ProjectID(ctx)))
}

func (s *PostgresStore) ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error) {
	query := `SELECT ` + pgReminderCols + ` FROM reminders WHERE user_id = $1 AND ($2 = 0 OR project_id = $2) AND cancelled = false`
	if activeOnly {
		query += ` AND enabled = true`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query, userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()
	return pgScanReminders(rows)
}

func (s *PostgresStore) ListEnabledForOwner(ctx context.Context, ownerID int64) ([]Reminder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+pgReminderCols+` FROM reminders WHERE user_id = $1 AND enabled = true AND cancelled = false`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list enabled reminders: %w", err)
	}
	defer rows.Close()
	return pgScanReminders(rows)
}

func (s *PostgresStore) UpdateReminder(ctx context.Context, userID, id int64, in ReminderInput) error {
	in.Title = s.enTitle(ctx, in.Title)
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET title = $1, message = $2, repeat_mode = $3, times = $4, weekdays = $5, day_of_month = $6, once_date = $7, event_at = $8, offsets = $9, enabled = $10
		 WHERE id = $11 AND user_id = $12 AND ($13 = 0 OR project_id = $13)`,
		in.Title, in.Title, in.RepeatMode, joinTimes(in.Times), joinInts(in.Weekdays), in.DayOfMonth, in.OnceDate, in.EventAt, joinInts(in.Offsets), in.Enabled,
		id, userID, authctx.ProjectID(ctx),
	)
	return err
}

// DeleteReminder soft-deletes (cancelled = true) so the calendar reconciler can
// remove any mirrored events before the row is finally removed.
func (s *PostgresStore) DeleteReminder(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE reminders SET cancelled = true, enabled = false WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, id, userID, authctx.ProjectID(ctx))
	return err
}

// HardDeleteReminder removes the row permanently (used by the reconciler after
// cleaning up any mirrored calendar events).
func (s *PostgresStore) HardDeleteReminder(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM reminders WHERE id = $1`, id)
	return err
}

// ListAllForOwner returns every reminder for a user, including disabled and
// cancelled ones (the reconciler needs those to clean up calendar events).
func (s *PostgresStore) ListAllForOwner(ctx context.Context, ownerID int64) ([]Reminder, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+pgReminderCols+` FROM reminders WHERE user_id = $1`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list all reminders: %w", err)
	}
	defer rows.Close()
	return pgScanReminders(rows)
}

// SetReminderCalendar records the mirrored calendar events for a reminder.
func (s *PostgresStore) SetReminderCalendar(ctx context.Context, id int64, conn string, eventIDs []string, hash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET calendar_conn = $1, calendar_event_ids = $2, calendar_hash = $3 WHERE id = $4`,
		conn, joinTimes(eventIDs), hash, id)
	return err
}

// ClearReminderCalendar forgets a reminder's calendar mirror (after deleting the events).
func (s *PostgresStore) ClearReminderCalendar(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET calendar_conn = '', calendar_event_ids = '', calendar_hash = '' WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) SetReminderEnabled(ctx context.Context, userID, id int64, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE reminders SET enabled = $1 WHERE id = $2 AND user_id = $3 AND ($4 = 0 OR project_id = $4)`, enabled, id, userID, authctx.ProjectID(ctx))
	return err
}

// MarkReminderFired advances last_fired_at and optionally disables a completed
// one-shot reminder, in one atomic statement.
func (s *PostgresStore) MarkReminderFired(ctx context.Context, id int64, firedAt time.Time, disable bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE reminders SET last_fired_at = $1, enabled = CASE WHEN $2 THEN false ELSE enabled END WHERE id = $3`,
		firedAt.UTC(), disable, id,
	)
	return err
}

func (s *PostgresStore) GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+pgReminderCols+`
		 FROM reminders
		 WHERE user_id = $1 AND ($3 = 0 OR project_id = $3) AND times = '' AND remind_at <= $2 AND notified = false AND cancelled = false
		 ORDER BY remind_at ASC`,
		userID, time.Now().UTC(), authctx.ProjectID(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("get due reminders: %w", err)
	}
	defer rows.Close()
	return pgScanReminders(rows)
}

func (s *PostgresStore) MarkReminderNotified(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE reminders SET notified = true WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) CancelReminder(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE reminders SET cancelled = true WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, id, userID, authctx.ProjectID(ctx))
	return err
}

// pgScanReminder scans a single reminder row. It deserializes the CSV-encoded
// slice columns with the shared join/split helpers and normalizes last_fired_at
// to UTC (nil when NULL), matching the SQLite scanReminder exactly. A no-rows
// result yields (nil, nil), preserving the SQLite GetReminder behavior.
func pgScanReminder(row pgx.Row) (*Reminder, error) {
	var r Reminder
	var times, weekdays, offsets, calEventIDs string
	var lastFired sql.NullTime
	err := row.Scan(
		&r.ID, &r.UserID, &r.Title, &r.Message, &r.RepeatMode, &times, &weekdays,
		&r.DayOfMonth, &r.OnceDate, &r.EventAt, &offsets, &r.Enabled, &lastFired,
		&r.CalendarConn, &calEventIDs, &r.CalendarHash,
		&r.RemindAt, &r.CreatedAt, &r.Notified, &r.Cancelled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan reminder: %w", err)
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

func pgScanReminders(rows pgx.Rows) ([]Reminder, error) {
	var reminders []Reminder
	for rows.Next() {
		r, err := pgScanReminder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminders = append(reminders, *r)
	}
	return reminders, rows.Err()
}
