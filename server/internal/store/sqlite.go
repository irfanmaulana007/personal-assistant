package store

import (
	"context"
	"database/sql"
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

		`CREATE TABLE IF NOT EXISTS llm_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			platform TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

// --- Reminders ---

func (s *SQLiteStore) CreateReminder(ctx context.Context, message string, remindAt time.Time) (*Reminder, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (message, remind_at, created_at) VALUES (?, ?, ?)`,
		message, remindAt.UTC(), time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Reminder{
		ID:        id,
		Message:   message,
		RemindAt:  remindAt,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (s *SQLiteStore) ListReminders(ctx context.Context, activeOnly bool) ([]Reminder, error) {
	query := `SELECT id, message, remind_at, created_at, notified, cancelled FROM reminders`
	if activeOnly {
		query += ` WHERE notified = 0 AND cancelled = 0`
	}
	query += ` ORDER BY remind_at ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()

	return scanReminders(rows)
}

func (s *SQLiteStore) GetDueReminders(ctx context.Context) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, message, remind_at, created_at, notified, cancelled
		 FROM reminders
		 WHERE remind_at <= ? AND notified = 0 AND cancelled = 0
		 ORDER BY remind_at ASC`,
		time.Now().UTC(),
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

func (s *SQLiteStore) CancelReminder(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET cancelled = 1 WHERE id = ?`, id)
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

func (s *SQLiteStore) CreateNote(ctx context.Context, title, content, tags string) (*Note, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (title, content, tags, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		title, content, tags, now, now,
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

func (s *SQLiteStore) GetNote(ctx context.Context, id int64) (*Note, error) {
	var n Note
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, content, tags, created_at, updated_at FROM notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	return &n, nil
}

func (s *SQLiteStore) UpdateNote(ctx context.Context, id int64, title, content, tags string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notes SET title = ?, content = ?, tags = ?, updated_at = ? WHERE id = ?`,
		title, content, tags, time.Now().UTC(), id,
	)
	return err
}

func (s *SQLiteStore) DeleteNote(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListNotes(ctx context.Context, tag string) ([]Note, error) {
	var rows *sql.Rows
	var err error

	if tag != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes
			 WHERE ',' || tags || ',' LIKE '%,' || ? || ',%'
			 ORDER BY updated_at DESC`, tag,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes ORDER BY updated_at DESC`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	return scanNotes(rows)
}

func (s *SQLiteStore) SearchNotes(ctx context.Context, query string) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.title, n.content, n.tags, n.created_at, n.updated_at
		 FROM notes n
		 JOIN notes_fts f ON n.id = f.rowid
		 WHERE notes_fts MATCH ?
		 ORDER BY rank`, query,
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
		`INSERT INTO message_log (platform, direction, sender, body, intent, action, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.Platform, log.Direction, log.Sender, log.Body, log.Intent, log.Action, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) GetMessageHistory(ctx context.Context, platform string, limit int) ([]MessageLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, platform, direction, sender, body, intent, action, created_at
		 FROM message_log
		 WHERE platform = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		platform, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get message history: %w", err)
	}
	defer rows.Close()

	var logs []MessageLog
	for rows.Next() {
		var l MessageLog
		if err := rows.Scan(&l.ID, &l.Platform, &l.Direction, &l.Sender, &l.Body, &l.Intent, &l.Action, &l.CreatedAt); err != nil {
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

// --- LLM Usage ---

func (s *SQLiteStore) LogUsage(ctx context.Context, usage *LLMUsage) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO llm_usage (model, prompt_tokens, completion_tokens, total_tokens, platform, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		usage.Model, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.Platform, time.Now().UTC(),
	)
	return err
}

// UsageStatsSince aggregates LLM token usage recorded at or after `since`.
func (s *SQLiteStore) UsageStatsSince(ctx context.Context, since time.Time) (*UsageStats, error) {
	sinceUTC := since.UTC()
	stats := &UsageStats{}

	// Summary
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(prompt_tokens), 0),
		        COALESCE(SUM(completion_tokens), 0),
		        COALESCE(SUM(total_tokens), 0)
		 FROM llm_usage WHERE created_at >= ?`, sinceUTC,
	).Scan(&stats.Summary.Requests, &stats.Summary.PromptTokens, &stats.Summary.CompletionTokens, &stats.Summary.TotalTokens)
	if err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}

	// By day
	dayRows, err := s.db.QueryContext(ctx,
		`SELECT date(created_at) AS d, COUNT(*), COALESCE(SUM(total_tokens), 0)
		 FROM llm_usage WHERE created_at >= ?
		 GROUP BY d ORDER BY d ASC`, sinceUTC,
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
		 FROM llm_usage WHERE created_at >= ?
		 GROUP BY model ORDER BY SUM(total_tokens) DESC`, sinceUTC,
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
	return stats, modelRows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
