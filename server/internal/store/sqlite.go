package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db *sql.DB
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
			category TEXT NOT NULL DEFAULT '',
			default_enabled INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
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
		{"reminders", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"notes", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"message_log", "user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"tool_usage", "user_id", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range addColumns {
		if err := s.addColumnIfMissing(c.table, c.column, c.ddl); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.table, c.column, err)
		}
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

func (s *SQLiteStore) CreateReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (user_id, message, remind_at, created_at) VALUES (?, ?, ?, ?)`,
		userID, message, remindAt.UTC(), now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Reminder{ID: id, Message: message, RemindAt: remindAt, CreatedAt: now}, nil
}

func (s *SQLiteStore) ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error) {
	query := `SELECT id, message, remind_at, created_at, notified, cancelled FROM reminders WHERE user_id = ?`
	if activeOnly {
		query += ` AND notified = 0 AND cancelled = 0`
	}
	query += ` ORDER BY remind_at ASC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()

	return scanReminders(rows)
}

func (s *SQLiteStore) GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, message, remind_at, created_at, notified, cancelled
		 FROM reminders
		 WHERE user_id = ? AND remind_at <= ? AND notified = 0 AND cancelled = 0
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

func scanReminders(rows *sql.Rows) ([]Reminder, error) {
	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		if err := rows.Scan(&r.ID, &r.Message, &r.RemindAt, &r.CreatedAt, &r.Notified, &r.Cancelled); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminders = append(reminders, r)
	}
	return reminders, rows.Err()
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
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.title, n.content, n.tags, n.created_at, n.updated_at
		 FROM notes n
		 JOIN notes_fts f ON n.id = f.rowid
		 WHERE notes_fts MATCH ? AND n.user_id = ?
		 ORDER BY rank`, query, userID,
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
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, platform, direction, sender, body, intent, action, created_at
		 FROM message_log
		 WHERE user_id = ? AND platform = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
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
	status := t.Status
	if status == "" {
		status = "ok"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO traces (user_id, platform, input, output, model, prompt_tokens, completion_tokens, total_tokens, latency_ms, tool_count, tools_json, status, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Platform, t.Input, t.Output, t.Model, t.PromptTokens, t.CompletionTokens, t.TotalTokens,
		t.LatencyMs, t.ToolCount, toolsJSON, status, t.Error, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert trace: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// UsageByDayModel returns per-day, per-model token sums for a cost time series.
func (s *SQLiteStore) UsageByDayModel(ctx context.Context, from, to time.Time, platform string) ([]DayModelUsage, error) {
	q := `SELECT date(created_at) AS d, model,
	             COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0)
	      FROM traces WHERE created_at >= ? AND created_at < ?`
	args := []any{from.UTC(), to.UTC()}
	if platform != "" {
		q += ` AND platform = ?`
		args = append(args, platform)
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
	return out, rows.Err()
}

func (s *SQLiteStore) ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error) {
	q := `SELECT id, user_id, platform, input, output, model, prompt_tokens, completion_tokens,
	             total_tokens, latency_ms, tool_count, status, error, created_at
	      FROM traces WHERE created_at >= ? AND created_at < ?`
	args := []any{f.From.UTC(), f.To.UTC()}
	if f.Platform != "" {
		q += ` AND platform = ?`
		args = append(args, f.Platform)
	}
	if f.Cursor > 0 {
		q += ` AND id < ?`
		args = append(args, f.Cursor)
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close()

	var traces []Trace
	for rows.Next() {
		var t Trace
		if err := rows.Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model,
			&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens, &t.LatencyMs, &t.ToolCount,
			&t.Status, &t.Error, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
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
	var toolsJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, platform, input, output, model, prompt_tokens, completion_tokens,
		        total_tokens, latency_ms, tool_count, tools_json, status, error, created_at
		 FROM traces WHERE id = ?`, id,
	).Scan(&t.ID, &t.UserID, &t.Platform, &t.Input, &t.Output, &t.Model,
		&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens, &t.LatencyMs, &t.ToolCount,
		&toolsJSON, &t.Status, &t.Error, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	if toolsJSON != "" {
		_ = json.Unmarshal([]byte(toolsJSON), &t.Tools)
	}
	return &t, nil
}

// UsageStatsBetween aggregates traces in the half-open interval [from, to),
// optionally restricted to a platform ("" = all).
func (s *SQLiteStore) UsageStatsBetween(ctx context.Context, from, to time.Time, platform string) (*UsageStats, error) {
	fromUTC := from.UTC()
	toUTC := to.UTC()
	stats := &UsageStats{}

	// Optional platform filter appended after the [from,to) args.
	pc := ""
	base := []any{fromUTC, toUTC}
	if platform != "" {
		pc = " AND platform = ?"
		base = append(base, platform)
	}

	// Summary
	var avgLatency sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(prompt_tokens), 0),
		        COALESCE(SUM(completion_tokens), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COALESCE(SUM(tool_count), 0),
		        COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		        AVG(NULLIF(latency_ms, 0))
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc, base...,
	).Scan(&stats.Summary.Requests, &stats.Summary.PromptTokens, &stats.Summary.CompletionTokens,
		&stats.Summary.TotalTokens, &stats.ToolCalls, &stats.Errors, &avgLatency)
	if err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}
	if avgLatency.Valid {
		stats.AvgLatencyMs = int(avgLatency.Float64)
	}

	// By day
	dayRows, err := s.db.QueryContext(ctx,
		`SELECT date(created_at) AS d, COUNT(*), COALESCE(SUM(total_tokens), 0)
		 FROM traces WHERE created_at >= ? AND created_at < ?`+pc+`
		 GROUP BY d ORDER BY d ASC`, base...,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by day: %w", err)
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var d UsageDay
		if err := dayRows.Scan(&d.Date, &d.Requests, &d.TotalTokens); err != nil {
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
	toolArgs := []any{fromUTC, toUTC}
	toolPC := ""
	if platform != "" {
		toolPC = " AND platform = ?"
		toolArgs = append(toolArgs, platform)
	}
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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
