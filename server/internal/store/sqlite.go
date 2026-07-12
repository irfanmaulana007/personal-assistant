package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore is the single-file backend that implements the full Store
// interface (both DataStore and LogStore halves) on its own. The hybrid backend
// splits these across PostgreSQL and MongoDB; this assertion keeps SQLite honest
// as the reference implementation.
var (
	_ Store     = (*SQLiteStore)(nil)
	_ DataStore = (*SQLiteStore)(nil)
	_ LogStore  = (*SQLiteStore)(nil)
)

type SQLiteStore struct {
	db *sql.DB
	// translator, when set, normalizes user text to English before persisting
	// reminders and bucket-list items. Optional — nil means store text as-is.
	translator Translator
}

// SetTranslator injects the English-normalization translator. It is wired after
// construction (in main) because the translator itself depends on settings,
// which read from this store.
func (s *SQLiteStore) SetTranslator(t Translator) {
	s.translator = t
}

// enTitle normalizes a title/name to English, or returns it unchanged when no
// translator is configured (e.g. in tests).
func (s *SQLiteStore) enTitle(ctx context.Context, text string) string {
	if s.translator == nil {
		return text
	}
	return s.translator.Title(ctx, text)
}

// enText normalizes free-form text to English, or returns it unchanged when no
// translator is configured.
func (s *SQLiteStore) enText(ctx context.Context, text string) string {
	if s.translator == nil {
		return text
	}
	return s.translator.Text(ctx, text)
}

func NewSQLite(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	// Rename life_goals -> bucket_list_items in place so existing data is kept.
	// Must run before the CREATE TABLE IF NOT EXISTS below, which would otherwise
	// create an empty bucket_list_items and block the rename.
	if err := s.renameTableIfNeeded("life_goals", "bucket_list_items"); err != nil {
		return fmt.Errorf("rename life_goals: %w", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			phone TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE INDEX IF NOT EXISTS idx_contacts_user ON contacts(user_id)`,

		`CREATE TABLE IF NOT EXISTS activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			occurred_at DATETIME NOT NULL,
			source TEXT NOT NULL DEFAULT 'chat',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE INDEX IF NOT EXISTS idx_activities_user ON activities(user_id, occurred_at)`,

		`CREATE TABLE IF NOT EXISTS bucket_list_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'other',
			resolution_year INTEGER,
			done INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			done_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bucket_list_items_user ON bucket_list_items(user_id, done)`,

		`CREATE TABLE IF NOT EXISTS trips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			destination TEXT NOT NULL DEFAULT '',
			budget REAL NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT '',
			active INTEGER NOT NULL DEFAULT 1,
			started_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE INDEX IF NOT EXISTS idx_trips_user ON trips(user_id, active)`,

		`CREATE TABLE IF NOT EXISTS trip_expenses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			trip_id INTEGER NOT NULL,
			amount REAL NOT NULL,
			currency TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'other',
			note TEXT NOT NULL DEFAULT '',
			spent_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE INDEX IF NOT EXISTS idx_trip_expenses ON trip_expenses(user_id, trip_id)`,

		`CREATE TABLE IF NOT EXISTS hike_mountains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hike_mountains_user ON hike_mountains(user_id)`,

		`CREATE TABLE IF NOT EXISTS hike_tracks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			mountain_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hike_tracks_user ON hike_tracks(user_id, mountain_id)`,

		`CREATE TABLE IF NOT EXISTS hike_participants (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hike_participants_user ON hike_participants(user_id)`,

		`CREATE TABLE IF NOT EXISTS hikes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			mountain_id INTEGER NOT NULL,
			camped INTEGER NOT NULL DEFAULT 0,
			up_track_id INTEGER NOT NULL DEFAULT 0,
			down_track_id INTEGER NOT NULL DEFAULT 0,
			days INTEGER NOT NULL DEFAULT 0,
			nights INTEGER NOT NULL DEFAULT 0,
			hiked_on DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hikes_user ON hikes(user_id, hiked_on)`,

		`CREATE TABLE IF NOT EXISTS hike_hikers (
			hike_id INTEGER NOT NULL,
			participant_id INTEGER NOT NULL,
			PRIMARY KEY (hike_id, participant_id)
		)`,

		`CREATE TABLE IF NOT EXISTS skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL DEFAULT '',
			tuned_prompt TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			default_enabled INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			prompt_updated_at DATETIME,
			prompt_updated_by TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS user_skills (
			user_id INTEGER NOT NULL,
			skill_id INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, skill_id)
		)`,

		`CREATE TABLE IF NOT EXISTS reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message TEXT NOT NULL,
			remind_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			notified BOOLEAN NOT NULL DEFAULT 0,
			cancelled BOOLEAN NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reminders_active ON reminders(remind_at) WHERE notified = 0 AND cancelled = 0`,

		`CREATE TABLE IF NOT EXISTS user_personas (
			user_id INTEGER PRIMARY KEY,
			tone TEXT NOT NULL DEFAULT 'balanced',
			emoji TEXT NOT NULL DEFAULT 'occasional',
			length TEXT NOT NULL DEFAULT 'balanced',
			personality TEXT NOT NULL DEFAULT 'balanced',
			name TEXT NOT NULL DEFAULT '',
			custom TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_user ON memories(user_id)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(content, content='memories', content_rowid='id')`,
		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.id, old.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.id, old.content);
			INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
		END`,

		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(title, content, tags, content='notes', content_rowid='id')`,

		// Triggers to keep FTS index in sync
		`CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
			INSERT INTO notes_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, content, tags) VALUES('delete', old.id, old.title, old.content, old.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, content, tags) VALUES('delete', old.id, old.title, old.content, old.tags);
			INSERT INTO notes_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags);
		END`,

		`CREATE TABLE IF NOT EXISTS message_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT NOT NULL,
			direction TEXT NOT NULL,
			sender TEXT NOT NULL,
			body TEXT NOT NULL,
			intent TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS oauth_tokens (
			service TEXT PRIMARY KEY,
			token_data BLOB NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value BLOB NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 0,
			platform TEXT NOT NULL DEFAULT '',
			input TEXT NOT NULL DEFAULT '',
			output TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			tool_count INTEGER NOT NULL DEFAULT 0,
			tools_json TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_created ON traces(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_user ON traces(user_id)`,

		`CREATE TABLE IF NOT EXISTS trace_scores (
			trace_id INTEGER PRIMARY KEY,
			accuracy INTEGER NOT NULL DEFAULT 0,
			helpfulness INTEGER NOT NULL DEFAULT 0,
			safety INTEGER NOT NULL DEFAULT 0,
			overall REAL NOT NULL DEFAULT 0,
			rationale TEXT NOT NULL DEFAULT '',
			judge_model TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_scores_overall ON trace_scores(overall)`,

		`CREATE TABLE IF NOT EXISTS model_prices (
			model TEXT PRIMARY KEY,
			input_per_1m REAL NOT NULL DEFAULT 0,
			output_per_1m REAL NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS tool_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tool TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, m)
		}
	}

	// Additive column migrations for tables created by earlier versions.
	addColumns := []struct{ table, column, ddl string }{
		{"users", "name", "TEXT NOT NULL DEFAULT ''"},
		{"skills", "tuned_prompt", "TEXT NOT NULL DEFAULT ''"},
		{"traces", "skills", "TEXT NOT NULL DEFAULT ''"},
		{"traces", "steps_json", "TEXT NOT NULL DEFAULT ''"},
		{"traces", "image_model", "TEXT NOT NULL DEFAULT ''"},
		{"traces", "image_prompt_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"traces", "image_completion_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"traces", "image_total_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"reminders", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"reminders", "title", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "enabled", "BOOLEAN NOT NULL DEFAULT 1"},
		{"reminders", "repeat_mode", "TEXT NOT NULL DEFAULT 'once'"},
		{"reminders", "times", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "weekdays", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "day_of_month", "INTEGER NOT NULL DEFAULT 0"},
		{"reminders", "once_date", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "event_at", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "offsets", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "last_fired_at", "DATETIME"},
		{"reminders", "calendar_conn", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "calendar_event_ids", "TEXT NOT NULL DEFAULT ''"},
		{"reminders", "calendar_hash", "TEXT NOT NULL DEFAULT ''"},
		{"bucket_list_items", "description", "TEXT NOT NULL DEFAULT ''"},
		{"bucket_list_items", "category", "TEXT NOT NULL DEFAULT 'other'"},
		{"bucket_list_items", "resolution_year", "INTEGER"},
		{"notes", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"message_log", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"tool_usage", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"skills", "prompt_updated_at", "DATETIME"},
		{"skills", "prompt_updated_by", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range addColumns {
		if err := s.addColumnIfMissing(c.table, c.column, c.ddl); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.table, c.column, err)
		}
	}

	// Resolution lookup index. Created after the columns above exist, because a
	// legacy life_goals table renamed to bucket_list_items gains resolution_year
	// only in the additive step just above.
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_bucket_list_items_resolution ON bucket_list_items(user_id, resolution_year)`,
	); err != nil {
		return fmt.Errorf("create idx_bucket_list_items_resolution: %w", err)
	}

	// Partial index for the scheduler's owner lookup. Created after the columns
	// above exist (the reminders table predates the `enabled` column).
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_reminders_owner_enabled ON reminders(user_id) WHERE enabled = 1 AND cancelled = 0`,
	); err != nil {
		return fmt.Errorf("create idx_reminders_owner_enabled: %w", err)
	}

	// Seed / upsert master data (skills).
	if err := s.seedSkills(); err != nil {
		return fmt.Errorf("seed skills: %w", err)
	}
	return nil
}

// addColumnIfMissing adds a column to a table only when it does not already
// exist (SQLite has no "ADD COLUMN IF NOT EXISTS").
func (s *SQLiteStore) addColumnIfMissing(table, column, ddl string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil // already present
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, ddl))
	return err
}

// renameTableIfNeeded renames oldName to newName only when oldName still exists
// and newName does not — so it runs exactly once and is a no-op afterwards.
func (s *SQLiteStore) renameTableIfNeeded(oldName, newName string) error {
	exists := func(name string) (bool, error) {
		var n int
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
		return n > 0, err
	}
	oldExists, err := exists(oldName)
	if err != nil {
		return err
	}
	newExists, err := exists(newName)
	if err != nil {
		return err
	}
	if !oldExists || newExists {
		return nil
	}
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName))
	return err
}

// --- Users ---

func (s *SQLiteStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *SQLiteStore) CreateUser(ctx context.Context, email, passwordHash, role string) (*User, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		email, passwordHash, role, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Email: email, PasswordHash: passwordHash, Role: role, CreatedAt: now}, nil
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE email = ?`, email))
}

func (s *SQLiteStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE id = ?`, id))
}

func (s *SQLiteStore) FirstAdmin(ctx context.Context) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1`))
}

func (s *SQLiteStore) scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) UpdateUserRole(ctx context.Context, id int64, role string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}

func (s *SQLiteStore) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

func (s *SQLiteStore) UpdateUserProfile(ctx context.Context, id int64, name, email string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET name = ?, email = ? WHERE id = ?`, name, email, id)
	return err
}

func (s *SQLiteStore) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// --- Reminders ---

const reminderCols = `id, user_id, title, message, repeat_mode, times, weekdays, day_of_month, once_date, event_at, offsets, enabled, last_fired_at, calendar_conn, calendar_event_ids, calendar_hash, remind_at, created_at, notified, cancelled`

func (s *SQLiteStore) CreateReminder(ctx context.Context, userID int64, in ReminderInput) (*Reminder, error) {
	in.Title = s.enTitle(ctx, in.Title)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (user_id, title, message, repeat_mode, times, weekdays, day_of_month, once_date, event_at, offsets, enabled, remind_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, in.Title, in.Title, in.RepeatMode,
		joinTimes(in.Times), joinInts(in.Weekdays), in.DayOfMonth, in.OnceDate, in.EventAt, joinInts(in.Offsets), in.Enabled,
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetReminder(ctx, userID, id)
}

// CreateLegacyReminder inserts a one-shot reminder for the natural-language chat
// path (no wall-clock recurrence semantics). Times stays empty so the scheduler
// uses the legacy remind_at branch.
func (s *SQLiteStore) CreateLegacyReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error) {
	message = s.enTitle(ctx, message)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (user_id, title, message, repeat_mode, remind_at, created_at) VALUES (?, ?, ?, 'once', ?, ?)`,
		userID, message, message, remindAt.UTC(), now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Reminder{ID: id, UserID: userID, Title: message, Message: message, RemindAt: remindAt, CreatedAt: now, Enabled: true}, nil
}

func (s *SQLiteStore) GetReminder(ctx context.Context, userID, id int64) (*Reminder, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+reminderCols+` FROM reminders WHERE id = ? AND user_id = ? AND cancelled = 0`, id, userID)
	r, err := scanReminder(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reminder: %w", err)
	}
	return r, nil
}

func (s *SQLiteStore) ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error) {
	query := `SELECT ` + reminderCols + ` FROM reminders WHERE user_id = ? AND cancelled = 0`
	if activeOnly {
		query += ` AND enabled = 1`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *SQLiteStore) ListEnabledForOwner(ctx context.Context, ownerID int64) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+reminderCols+` FROM reminders WHERE user_id = ? AND enabled = 1 AND cancelled = 0`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list enabled reminders: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *SQLiteStore) UpdateReminder(ctx context.Context, userID, id int64, in ReminderInput) error {
	in.Title = s.enTitle(ctx, in.Title)
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET title = ?, message = ?, repeat_mode = ?, times = ?, weekdays = ?, day_of_month = ?, once_date = ?, event_at = ?, offsets = ?, enabled = ?
		 WHERE id = ? AND user_id = ?`,
		in.Title, in.Title, in.RepeatMode, joinTimes(in.Times), joinInts(in.Weekdays), in.DayOfMonth, in.OnceDate, in.EventAt, joinInts(in.Offsets), in.Enabled,
		id, userID,
	)
	return err
}

// DeleteReminder soft-deletes (cancelled = 1) so the calendar reconciler can
// remove any mirrored events before the row is finally removed.
func (s *SQLiteStore) DeleteReminder(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET cancelled = 1, enabled = 0 WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// HardDeleteReminder removes the row permanently (used by the reconciler after
// cleaning up any mirrored calendar events).
func (s *SQLiteStore) HardDeleteReminder(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ?`, id)
	return err
}

// ListAllForOwner returns every reminder for a user, including disabled and
// cancelled ones (the reconciler needs those to clean up calendar events).
func (s *SQLiteStore) ListAllForOwner(ctx context.Context, ownerID int64) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+reminderCols+` FROM reminders WHERE user_id = ?`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list all reminders: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

// SetReminderCalendar records the mirrored calendar events for a reminder.
func (s *SQLiteStore) SetReminderCalendar(ctx context.Context, id int64, conn string, eventIDs []string, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET calendar_conn = ?, calendar_event_ids = ?, calendar_hash = ? WHERE id = ?`,
		conn, joinTimes(eventIDs), hash, id)
	return err
}

// ClearReminderCalendar forgets a reminder's calendar mirror (after deleting the events).
func (s *SQLiteStore) ClearReminderCalendar(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET calendar_conn = '', calendar_event_ids = '', calendar_hash = '' WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) SetReminderEnabled(ctx context.Context, userID, id int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET enabled = ? WHERE id = ? AND user_id = ?`, enabled, id, userID)
	return err
}

// MarkReminderFired advances last_fired_at and optionally disables a completed
// one-shot reminder, in one atomic statement.
func (s *SQLiteStore) MarkReminderFired(ctx context.Context, id int64, firedAt time.Time, disable bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET last_fired_at = ?, enabled = CASE WHEN ? THEN 0 ELSE enabled END WHERE id = ?`,
		firedAt.UTC(), disable, id,
	)
	return err
}

func (s *SQLiteStore) GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+reminderCols+`
		 FROM reminders
		 WHERE user_id = ? AND times = '' AND remind_at <= ? AND notified = 0 AND cancelled = 0
		 ORDER BY remind_at ASC`,
		userID, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("get due reminders: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *SQLiteStore) MarkReminderNotified(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET notified = 1 WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) CancelReminder(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET cancelled = 1 WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

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

func (s *SQLiteStore) CreateNote(ctx context.Context, userID int64, title, content, tags string) (*Note, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (user_id, title, content, tags, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, title, content, tags, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Note{
		ID:        id,
		Title:     title,
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *SQLiteStore) GetNote(ctx context.Context, userID, id int64) (*Note, error) {
	var n Note
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, content, tags, created_at, updated_at FROM notes WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	return &n, nil
}

func (s *SQLiteStore) UpdateNote(ctx context.Context, userID, id int64, title, content, tags string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notes SET title = ?, content = ?, tags = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		title, content, tags, time.Now().UTC(), id, userID,
	)
	return err
}

func (s *SQLiteStore) DeleteNote(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *SQLiteStore) ListNotes(ctx context.Context, userID int64, tag string) ([]Note, error) {
	var rows *sql.Rows
	var err error

	if tag != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes
			 WHERE user_id = ? AND ',' || tags || ',' LIKE '%,' || ? || ',%'
			 ORDER BY updated_at DESC`, userID, tag,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes WHERE user_id = ? ORDER BY updated_at DESC`,
			userID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	return scanNotes(rows)
}

func (s *SQLiteStore) SearchNotes(ctx context.Context, userID int64, query string) ([]Note, error) {
	ftsQuery := sqliteFTS5Query(query)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.title, n.content, n.tags, n.created_at, n.updated_at
		 FROM notes n
		 JOIN notes_fts f ON n.id = f.rowid
		 WHERE notes_fts MATCH ? AND n.user_id = ?
		 ORDER BY rank`, ftsQuery, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer rows.Close()

	return scanNotes(rows)
}

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

// --- Message Log ---

func (s *SQLiteStore) LogMessage(ctx context.Context, log *MessageLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO message_log (user_id, platform, direction, sender, body, intent, action, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.UserID, log.Platform, log.Direction, log.Sender, log.Body, log.Intent, log.Action, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error) {
	// Take the most-recent `limit` rows, then present them oldest-first.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, platform, direction, sender, body, intent, action, created_at FROM (
		   SELECT id, user_id, platform, direction, sender, body, intent, action, created_at
		   FROM message_log
		   WHERE user_id = ? AND platform = ?
		   ORDER BY created_at DESC, id DESC
		   LIMIT ?
		 ) ORDER BY created_at ASC, id ASC`,
		userID, platform, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get message history: %w", err)
	}
	defer rows.Close()

	var logs []MessageLog
	for rows.Next() {
		var l MessageLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Platform, &l.Direction, &l.Sender, &l.Body, &l.Intent, &l.Action, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// --- OAuth Tokens ---

func (s *SQLiteStore) SaveToken(ctx context.Context, service string, tokenData []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (service, token_data, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(service) DO UPDATE SET token_data = excluded.token_data, updated_at = excluded.updated_at`,
		service, tokenData, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) GetToken(ctx context.Context, service string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT token_data FROM oauth_tokens WHERE service = ?`, service,
	).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

// --- Settings ---

func (s *SQLiteStore) GetSetting(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return value, err
}

func (s *SQLiteStore) SetSetting(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) GetAllSettings(ctx context.Context) (map[string][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

// --- Tool usage ---

func (s *SQLiteStore) LogToolUsage(ctx context.Context, userID int64, tool, platform string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tool_usage (user_id, tool, platform, created_at) VALUES (?, ?, ?, ?)`,
		userID, tool, platform, time.Now().UTC(),
	)
	return err
}

// --- Traces ---

func (s *SQLiteStore) CreateTrace(ctx context.Context, t *Trace) (int64, error) {
	toolsJSON := "[]"
	if len(t.Tools) > 0 {
		if b, err := json.Marshal(t.Tools); err == nil {
			toolsJSON = string(b)
		}
	}
	stepsJSON := ""
	if len(t.Steps) > 0 {
		if b, err := json.Marshal(t.Steps); err == nil {
			stepsJSON = string(b)
		}
	}
	status := t.Status
	if status == "" {
		status = "ok"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO traces (user_id, platform, input, output, model, prompt_tokens, completion_tokens, total_tokens, image_model, image_prompt_tokens, image_completion_tokens, image_total_tokens, latency_ms, tool_count, tools_json, skills, steps_json, status, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Platform, t.Input, t.Output, t.Model, t.PromptTokens, t.CompletionTokens, t.TotalTokens,
		t.ImageModel, t.ImagePromptTokens, t.ImageCompletionTokens, t.ImageTotalTokens,
		t.LatencyMs, t.ToolCount, toolsJSON, strings.Join(t.Skills, ","), stepsJSON, status, t.Error, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert trace: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// sqliteInClause builds an ` AND <col> IN (?, ?, …)` fragment and its args for a
// non-empty value list. Returns ("", nil) when vals is empty, i.e. "all".
func sqliteInClause(col string, vals []string) (string, []any) {
	if len(vals) == 0 {
		return "", nil
	}
	ph := make([]string, len(vals))
	args := make([]any, len(vals))
	for i, v := range vals {
		ph[i] = "?"
		args[i] = v
	}
	return " AND " + col + " IN (" + strings.Join(ph, ", ") + ")", args
}

// UsageByDayModel returns per-day, per-model token sums for a cost time series.
func (s *SQLiteStore) UsageByDayModel(ctx context.Context, from, to time.Time, platforms []string) ([]DayModelUsage, error) {
	q := `SELECT date(created_at) AS d, model,
	             COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0)
	      FROM traces WHERE created_at >= ? AND created_at < ?`
	args := []any{from.UTC(), to.UTC()}
	if clause, a := sqliteInClause("platform", platforms); clause != "" {
		q += clause
		args = append(args, a...)
	}
	q += ` GROUP BY d, model ORDER BY d ASC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by day/model: %w", err)
	}
	defer rows.Close()

	var out []DayModelUsage
	for rows.Next() {
		var u DayModelUsage
		if err := rows.Scan(&u.Date, &u.Model, &u.PromptTokens, &u.CompletionTokens); err != nil {
			return nil, fmt.Errorf("scan day/model: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Image-generation usage (gpt-image-1) contributes its own per-day rows, so
	// the by-day cost series priced in the API layer covers LLM + image combined.
	iq := `SELECT date(created_at) AS d, image_model,
	              COALESCE(SUM(image_prompt_tokens), 0), COALESCE(SUM(image_completion_tokens), 0)
	       FROM traces WHERE created_at >= ? AND created_at < ? AND image_total_tokens > 0`
	iargs := []any{from.UTC(), to.UTC()}
	if clause, a := sqliteInClause("platform", platforms); clause != "" {
		iq += clause
		iargs = append(iargs, a...)
	}
	iq += ` GROUP BY d, image_model ORDER BY d ASC`
	irows, err := s.db.QueryContext(ctx, iq, iargs...)
	if err != nil {
		return nil, fmt.Errorf("usage image by day/model: %w", err)
	}
	defer irows.Close()
	for irows.Next() {
		var u DayModelUsage
		if err := irows.Scan(&u.Date, &u.Model, &u.PromptTokens, &u.CompletionTokens); err != nil {
			return nil, fmt.Errorf("scan image day/model: %w", err)
		}
		out = append(out, u)
	}
	return out, irows.Err()
}

func (s *SQLiteStore) ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error) {
	q := `SELECT t.id, t.user_id, t.platform, t.input, t.output, t.model, t.prompt_tokens,
	             t.completion_tokens, t.total_tokens,
	             t.image_model, t.image_prompt_tokens, t.image_completion_tokens, t.image_total_tokens,
	             t.latency_ms, t.tool_count, t.status, t.error, t.created_at,
	             sc.accuracy, sc.helpfulness, sc.safety, sc.overall, sc.rationale, sc.judge_model
	      FROM traces t
	      LEFT JOIN trace_scores sc ON sc.trace_id = t.id
	      WHERE t.created_at >= ? AND t.created_at < ?`
	args := []any{f.From.UTC(), f.To.UTC()}
	if clause, a := sqliteInClause("t.platform", f.Platforms); clause != "" {
		q += clause
		args = append(args, a...)
	}
	// Each selected score state is an OR-branch; a trace matches if it is in ANY
	// of the states. Empty = all. (The `low` branch carries a threshold arg,
	// appended in the same order its placeholder appears in the query.)
	var scoreOr []string
	for _, st := range f.ScoreStates {
		switch st {
		case "scored":
			scoreOr = append(scoreOr, `sc.trace_id IS NOT NULL`)
		case "unscored":
			// Only judgeable replies (successful, non-empty) count as "unscored" —
			// error traces are never judged, so surfacing them here would be noise.
			scoreOr = append(scoreOr, `(sc.trace_id IS NULL AND t.status = 'ok' AND t.output != '')`)
		case "low":
			scoreOr = append(scoreOr, `(sc.trace_id IS NOT NULL AND sc.overall < ?)`)
			args = append(args, LowScoreThreshold)
		}
	}
	if len(scoreOr) > 0 {
		q += ` AND (` + strings.Join(scoreOr, " OR ") + `)`
	}
	if f.Cursor > 0 {
		q += ` AND t.id < ?`
		args = append(args, f.Cursor)
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q += ` ORDER BY t.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close()

	var traces []Trace
	for rows.Next() {
		var t Trace
		var acc, help, safe sql.NullInt64
		var overall sql.NullFloat64
		var rationale, judgeModel sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model,
			&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
			&t.ImageModel, &t.ImagePromptTokens, &t.ImageCompletionTokens, &t.ImageTotalTokens,
			&t.LatencyMs, &t.ToolCount,
			&t.Status, &t.Error, &t.CreatedAt,
			&acc, &help, &safe, &overall, &rationale, &judgeModel); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		if overall.Valid {
			t.Score = &TraceScore{
				TraceID:     t.ID,
				Accuracy:    int(acc.Int64),
				Helpfulness: int(help.Int64),
				Safety:      int(safe.Int64),
				Overall:     overall.Float64,
				Rationale:   rationale.String,
				JudgeModel:  judgeModel.String,
			}
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// ListLowScoreTracesWithSkills mirrors the Mongo implementation for the legacy
// SQLite backend (used by the migrate-db ETL, not the live app). The SQLite
// traces table has no `source` column, so excludeSources is ignored here.
func (s *SQLiteStore) ListLowScoreTracesWithSkills(ctx context.Context, userID int64, from, to time.Time, maxOverall float64, excludeSources []string, limit int) ([]Trace, error) {
	q := `SELECT t.id, t.user_id, t.platform, t.input, t.output, t.model, t.latency_ms,
	             t.tool_count, t.tools_json, t.skills, t.status, t.error, t.created_at,
	             sc.accuracy, sc.helpfulness, sc.safety, sc.overall, sc.rationale, sc.judge_model
	      FROM traces t
	      JOIN trace_scores sc ON sc.trace_id = t.id
	      WHERE t.user_id = ? AND t.created_at >= ? AND t.created_at < ?
	        AND sc.overall <= ? AND t.skills != ''`
	args := []any{userID, from.UTC(), to.UTC(), maxOverall}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q += ` ORDER BY sc.overall ASC, t.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list low-score traces: %w", err)
	}
	defer rows.Close()

	var out []Trace
	for rows.Next() {
		var t Trace
		var toolsJSON, skills string
		var acc, help, safe sql.NullInt64
		var overall sql.NullFloat64
		var rationale, judgeModel sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model, &t.LatencyMs,
			&t.ToolCount, &toolsJSON, &skills, &t.Status, &t.Error, &t.CreatedAt,
			&acc, &help, &safe, &overall, &rationale, &judgeModel); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		if toolsJSON != "" {
			_ = json.Unmarshal([]byte(toolsJSON), &t.Tools)
		}
		if skills != "" {
			t.Skills = strings.Split(skills, ",")
		}
		if overall.Valid {
			t.Score = &TraceScore{
				TraceID: t.ID, Accuracy: int(acc.Int64), Helpfulness: int(help.Int64),
				Safety: int(safe.Int64), Overall: overall.Float64,
				Rationale: rationale.String, JudgeModel: judgeModel.String,
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetUserActivity returns per-user counts for the profile page.
func (s *SQLiteStore) GetUserActivity(ctx context.Context, userID int64) (*UserActivity, error) {
	a := &UserActivity{}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM traces WHERE user_id = ?`, userID,
	).Scan(&a.Runs, &a.TotalTokens); err != nil {
		return nil, fmt.Errorf("user runs: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reminders WHERE user_id = ? AND notified = 0 AND cancelled = 0`, userID,
	).Scan(&a.Reminders); err != nil {
		return nil, fmt.Errorf("user reminders: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notes WHERE user_id = ?`, userID,
	).Scan(&a.Notes); err != nil {
		return nil, fmt.Errorf("user notes: %w", err)
	}
	return a, nil
}

func (s *SQLiteStore) GetTrace(ctx context.Context, id int64) (*Trace, error) {
	var t Trace
	var toolsJSON, stepsJSON, skills string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, platform, input, output, model, prompt_tokens, completion_tokens,
		        total_tokens, image_model, image_prompt_tokens, image_completion_tokens, image_total_tokens,
		        latency_ms, tool_count, tools_json, skills, steps_json, status, error, created_at
		 FROM traces WHERE id = ?`, id,
	).Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model,
		&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
		&t.ImageModel, &t.ImagePromptTokens, &t.ImageCompletionTokens, &t.ImageTotalTokens,
		&t.LatencyMs, &t.ToolCount,
		&toolsJSON, &skills, &stepsJSON, &t.Status, &t.Error, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	if toolsJSON != "" {
		_ = json.Unmarshal([]byte(toolsJSON), &t.Tools)
	}
	if stepsJSON != "" {
		_ = json.Unmarshal([]byte(stepsJSON), &t.Steps)
	}
	if skills != "" {
		t.Skills = strings.Split(skills, ",")
	}
	if sc, err := s.GetTraceScore(ctx, t.ID); err == nil {
		t.Score = sc
	}
	return &t, nil
}

// --- Trace scores (LLM-as-judge) ---

// SaveTraceScore upserts the judge verdict for a trace (one score per trace).
func (s *SQLiteStore) SaveTraceScore(ctx context.Context, sc *TraceScore) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trace_scores (trace_id, accuracy, helpfulness, safety, overall, rationale, judge_model, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(trace_id) DO UPDATE SET
		   accuracy=excluded.accuracy, helpfulness=excluded.helpfulness, safety=excluded.safety,
		   overall=excluded.overall, rationale=excluded.rationale, judge_model=excluded.judge_model,
		   created_at=excluded.created_at`,
		sc.TraceID, sc.Accuracy, sc.Helpfulness, sc.Safety, sc.Overall, sc.Rationale, sc.JudgeModel, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save trace score: %w", err)
	}
	return nil
}

// GetTraceScore returns the score for a trace, or nil if it hasn't been judged.
func (s *SQLiteStore) GetTraceScore(ctx context.Context, traceID int64) (*TraceScore, error) {
	var sc TraceScore
	err := s.db.QueryRowContext(ctx,
		`SELECT trace_id, accuracy, helpfulness, safety, overall, rationale, judge_model, created_at
		 FROM trace_scores WHERE trace_id = ?`, traceID,
	).Scan(&sc.TraceID, &sc.Accuracy, &sc.Helpfulness, &sc.Safety, &sc.Overall, &sc.Rationale, &sc.JudgeModel, &sc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace score: %w", err)
	}
	return &sc, nil
}

// ListUnscoredTraces returns successful traces created at/after since that have
// no score yet, oldest first, capped at limit. Error traces are skipped — there
// is no useful reply to judge.
func (s *SQLiteStore) ListUnscoredTraces(ctx context.Context, since time.Time, limit int) ([]Trace, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.user_id, t.platform, t.input, t.output, t.model, t.prompt_tokens,
		        t.completion_tokens, t.total_tokens, t.latency_ms, t.tool_count, t.status, t.error, t.created_at
		 FROM traces t
		 LEFT JOIN trace_scores sc ON sc.trace_id = t.id
		 WHERE sc.trace_id IS NULL AND t.status = 'ok' AND t.output != '' AND t.created_at >= ?
		 ORDER BY t.id ASC LIMIT ?`, since.UTC(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unscored traces: %w", err)
	}
	defer rows.Close()
	var traces []Trace
	for rows.Next() {
		var t Trace
		if err := rows.Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model,
			&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens, &t.LatencyMs, &t.ToolCount,
			&t.Status, &t.Error, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan unscored trace: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// UsageStatsBetween aggregates traces in the half-open interval [from, to),
// optionally restricted to a platform ("" = all).
func (s *SQLiteStore) UsageStatsBetween(ctx context.Context, from, to time.Time, platforms []string) (*UsageStats, error) {
	fromUTC := from.UTC()
	toUTC := to.UTC()
	stats := &UsageStats{}

	// Optional platform filter appended after the [from,to) args.
	pc, pcArgs := sqliteInClause("platform", platforms)
	base := append([]any{fromUTC, toUTC}, pcArgs...)

	// Summary
	var avgLatency sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(prompt_tokens), 0),
		        COALESCE(SUM(completion_tokens), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COALESCE(SUM(tool_count), 0),
		        COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		        COUNT(DISTINCT user_id),
		        AVG(NULLIF(latency_ms, 0))
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc, base...,
	).Scan(&stats.Summary.Requests, &stats.Summary.PromptTokens, &stats.Summary.CompletionTokens,
		&stats.Summary.TotalTokens, &stats.ToolCalls, &stats.Errors, &stats.ActiveUsers, &avgLatency)
	if err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}
	if avgLatency.Valid {
		stats.AvgLatencyMs = int(avgLatency.Float64)
	}

	// Latency percentiles (loads the latency column for the range; fine at this scale).
	if p50, p95, p99, err := s.latencyPercentiles(ctx, pc, base); err != nil {
		return nil, err
	} else {
		stats.LatencyP50, stats.LatencyP95, stats.LatencyP99 = p50, p95, p99
	}

	// Requests by hour-of-day and day-of-week (UTC; client rotates by display tz).
	if err := s.usageByBucket(ctx, "%H", pc, base, stats.ByHour[:]); err != nil {
		return nil, fmt.Errorf("usage by hour: %w", err)
	}
	if err := s.usageByBucket(ctx, "%w", pc, base, stats.ByWeekday[:]); err != nil {
		return nil, fmt.Errorf("usage by weekday: %w", err)
	}

	// By day
	dayRows, err := s.db.QueryContext(ctx,
		`SELECT date(created_at) AS d, COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COALESCE(CAST(AVG(NULLIF(latency_ms, 0)) AS INTEGER), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc+`
		 GROUP BY d ORDER BY d ASC`, base...,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by day: %w", err)
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var d UsageDay
		if err := dayRows.Scan(&d.Date, &d.Requests, &d.Errors, &d.TotalTokens, &d.AvgLatencyMs); err != nil {
			return nil, fmt.Errorf("scan usage day: %w", err)
		}
		stats.ByDay = append(stats.ByDay, d)
	}
	if err := dayRows.Err(); err != nil {
		return nil, err
	}

	// By model
	modelRows, err := s.db.QueryContext(ctx,
		`SELECT model, COUNT(*),
		        COALESCE(SUM(prompt_tokens), 0),
		        COALESCE(SUM(completion_tokens), 0),
		        COALESCE(SUM(total_tokens), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc+`
		 GROUP BY model ORDER BY SUM(total_tokens) DESC`, base...,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by model: %w", err)
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var m UsageModel
		if err := modelRows.Scan(&m.Model, &m.Requests, &m.PromptTokens, &m.CompletionTokens, &m.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan usage model: %w", err)
		}
		stats.ByModel = append(stats.ByModel, m)
	}
	if err := modelRows.Err(); err != nil {
		return nil, err
	}

	// Fold image-generation usage (gpt-image-1) into the aggregates as its own
	// model. Image tokens live in dedicated columns and are priced with a much
	// higher rate, so they are summed here rather than through the LLM columns
	// above: this keeps the summary and per-day token totals combined (LLM +
	// image) while the by-model breakdown still shows the two apart.
	imgModelRows, err := s.db.QueryContext(ctx,
		`SELECT image_model, COUNT(*),
		        COALESCE(SUM(image_prompt_tokens), 0),
		        COALESCE(SUM(image_completion_tokens), 0),
		        COALESCE(SUM(image_total_tokens), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ? AND image_total_tokens > 0`+pc+`
		 GROUP BY image_model ORDER BY SUM(image_total_tokens) DESC`, base...,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by image model: %w", err)
	}
	defer imgModelRows.Close()
	for imgModelRows.Next() {
		var m UsageModel
		if err := imgModelRows.Scan(&m.Model, &m.Requests, &m.PromptTokens, &m.CompletionTokens, &m.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan image model: %w", err)
		}
		stats.ByModel = append(stats.ByModel, m)
		stats.Summary.PromptTokens += m.PromptTokens
		stats.Summary.CompletionTokens += m.CompletionTokens
		stats.Summary.TotalTokens += m.TotalTokens
	}
	if err := imgModelRows.Err(); err != nil {
		return nil, err
	}

	// Image tokens per day, added onto the combined by-day token series so the
	// tokens line tracks the (combined) cost line on the dashboard.
	imgDayRows, err := s.db.QueryContext(ctx,
		`SELECT date(created_at) AS d, COALESCE(SUM(image_total_tokens), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ? AND image_total_tokens > 0`+pc+`
		 GROUP BY d`, base...,
	)
	if err != nil {
		return nil, fmt.Errorf("usage image by day: %w", err)
	}
	defer imgDayRows.Close()
	imgByDay := map[string]int{}
	for imgDayRows.Next() {
		var day string
		var tok int
		if err := imgDayRows.Scan(&day, &tok); err != nil {
			return nil, fmt.Errorf("scan image day: %w", err)
		}
		imgByDay[day] = tok
	}
	if err := imgDayRows.Err(); err != nil {
		return nil, err
	}
	for i := range stats.ByDay {
		stats.ByDay[i].TotalTokens += imgByDay[stats.ByDay[i].Date]
	}

	// By platform (ignores the platform filter so the split is always visible)
	platRows, err := s.db.QueryContext(ctx,
		`SELECT platform, COUNT(*), COALESCE(SUM(total_tokens), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ?
		 GROUP BY platform ORDER BY COUNT(*) DESC`, fromUTC, toUTC,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by platform: %w", err)
	}
	defer platRows.Close()
	for platRows.Next() {
		var p UsagePlatform
		if err := platRows.Scan(&p.Platform, &p.Requests, &p.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan usage platform: %w", err)
		}
		if p.Platform == "" {
			p.Platform = "unknown"
		}
		stats.ByPlatform = append(stats.ByPlatform, p)
	}
	if err := platRows.Err(); err != nil {
		return nil, err
	}

	// Top tools
	toolPC, toolPCArgs := sqliteInClause("platform", platforms)
	toolArgs := append([]any{fromUTC, toUTC}, toolPCArgs...)
	toolRows, err := s.db.QueryContext(ctx,
		`SELECT tool, COUNT(*) AS c
		 FROM tool_usage WHERE created_at >= ? AND created_at < ?`+toolPC+`
		 GROUP BY tool ORDER BY c DESC LIMIT 10`, toolArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("top tools: %w", err)
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var t ToolCount
		if err := toolRows.Scan(&t.Tool, &t.Count); err != nil {
			return nil, fmt.Errorf("scan tool count: %w", err)
		}
		stats.TopTools = append(stats.TopTools, t)
	}
	return stats, toolRows.Err()
}

// latencyPercentiles loads latency_ms for the range and returns p50/p95/p99.
func (s *SQLiteStore) latencyPercentiles(ctx context.Context, pc string, base []any) (p50, p95, p99 int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT latency_ms FROM traces WHERE created_at >= ? AND created_at < ? AND latency_ms > 0`+pc+`
		 ORDER BY latency_ms ASC`, base...)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("latency percentiles: %w", err)
	}
	defer rows.Close()
	var lat []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return 0, 0, 0, err
		}
		lat = append(lat, v)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	pick := func(p float64) int {
		if len(lat) == 0 {
			return 0
		}
		idx := int(math.Ceil(p*float64(len(lat)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(lat) {
			idx = len(lat) - 1
		}
		return lat[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99), nil
}

// usageByBucket fills out[] with request counts grouped by a strftime bucket
// (e.g. "%H" for hour-of-day 0..23, "%w" for weekday 0..6, Sunday=0), in UTC.
func (s *SQLiteStore) usageByBucket(ctx context.Context, strftimeFmt, pc string, base []any, out []int) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(strftime('`+strftimeFmt+`', created_at) AS INTEGER) AS b, COUNT(*)
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc+`
		 GROUP BY b`, base...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var b, c int
		if err := rows.Scan(&b, &c); err != nil {
			return err
		}
		if b >= 0 && b < len(out) {
			out[b] = c
		}
	}
	return rows.Err()
}

// UsageByUserModel returns per-user, per-model usage for the Users section
// (cost is priced in the API layer, like UsageByDayModel).
func (s *SQLiteStore) UsageByUserModel(ctx context.Context, from, to time.Time, platforms []string) ([]UserModelUsage, error) {
	q := `SELECT user_id, model, COUNT(*),
	             COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0),
	             COALESCE(SUM(total_tokens), 0),
	             COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0)
	      FROM traces WHERE created_at >= ? AND created_at < ?`
	args := []any{from.UTC(), to.UTC()}
	if clause, a := sqliteInClause("platform", platforms); clause != "" {
		q += clause
		args = append(args, a...)
	}
	q += ` GROUP BY user_id, model`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by user/model: %w", err)
	}
	defer rows.Close()

	var out []UserModelUsage
	for rows.Next() {
		var u UserModelUsage
		if err := rows.Scan(&u.UserID, &u.Model, &u.Requests, &u.PromptTokens,
			&u.CompletionTokens, &u.TotalTokens, &u.Errors); err != nil {
			return nil, fmt.Errorf("scan user/model: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Image-generation usage (gpt-image-1) as its own per-user rows so per-user
	// cost includes image generation. Requests/Errors are left at zero: the run
	// is already counted by its LLM row above, so counting it again here would
	// double the user's request total.
	iq := `SELECT user_id, image_model,
	              COALESCE(SUM(image_prompt_tokens), 0), COALESCE(SUM(image_completion_tokens), 0),
	              COALESCE(SUM(image_total_tokens), 0)
	       FROM traces WHERE created_at >= ? AND created_at < ? AND image_total_tokens > 0`
	iargs := []any{from.UTC(), to.UTC()}
	if clause, a := sqliteInClause("platform", platforms); clause != "" {
		iq += clause
		iargs = append(iargs, a...)
	}
	iq += ` GROUP BY user_id, image_model`
	irows, err := s.db.QueryContext(ctx, iq, iargs...)
	if err != nil {
		return nil, fmt.Errorf("usage image by user/model: %w", err)
	}
	defer irows.Close()
	for irows.Next() {
		var u UserModelUsage
		if err := irows.Scan(&u.UserID, &u.Model, &u.PromptTokens, &u.CompletionTokens, &u.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan image user/model: %w", err)
		}
		out = append(out, u)
	}
	return out, irows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
