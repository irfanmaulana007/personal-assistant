package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *PostgresStore) CreateMemory(ctx context.Context, userID int64, content, kind string) (*Memory, error) {
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO memories (user_id, content, kind, created_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, content, kind, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}
	return &Memory{ID: id, Content: content, Kind: kind, CreatedAt: now}, nil
}

// SearchMemories returns the user's memories matching arbitrary query text,
// best-match first. The raw text is handed to Postgres' websearch_to_tsquery on
// the generated tsvector column; an empty or whitespace-only query returns no
// results.
func (s *PostgresStore) SearchMemories(ctx context.Context, userID int64, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 6
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, content, kind, created_at FROM memories
		 WHERE search @@ websearch_to_tsquery('simple', $1) AND user_id = $2
		 ORDER BY ts_rank(search, websearch_to_tsquery('simple', $1)) DESC
		 LIMIT $3`, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// ListMemories returns the user's most recent memories.
func (s *PostgresStore) ListMemories(ctx context.Context, userID int64, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, content, kind, created_at FROM memories
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *PostgresStore) DeleteMemory(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM memories WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}
