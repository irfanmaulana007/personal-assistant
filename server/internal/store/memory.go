package store

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateMemory(ctx context.Context, userID int64, content, kind string) (*Memory, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (user_id, content, kind, created_at) VALUES (?, ?, ?, ?)`,
		userID, content, kind, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Memory{ID: id, Content: content, Kind: kind, CreatedAt: now}, nil
}

// SearchMemories returns the user's memories matching an FTS5 query, best-match
// first. The caller must pass a sanitized FTS query.
func (s *SQLiteStore) SearchMemories(ctx context.Context, userID int64, ftsQuery string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 6
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.content, m.kind, m.created_at
		 FROM memories m JOIN memories_fts f ON m.id = f.rowid
		 WHERE memories_fts MATCH ? AND m.user_id = ?
		 ORDER BY rank LIMIT ?`, ftsQuery, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// ListMemories returns the user's most recent memories.
func (s *SQLiteStore) ListMemories(ctx context.Context, userID int64, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, kind, created_at FROM memories
		 WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *SQLiteStore) DeleteMemory(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

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
