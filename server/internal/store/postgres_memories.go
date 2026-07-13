package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
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
// best-match first. Recall is deliberately broad: the raw text is reduced to its
// significant tokens which are OR-joined into a tsquery, so a natural-language
// question ("what's my japan budget?") still surfaces memories that contain any
// of those terms. An empty or all-noise query returns no results.
func (s *PostgresStore) SearchMemories(ctx context.Context, userID int64, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 6
	}
	tsq := memoryTsquery(query)
	if tsq == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, content, kind, created_at FROM memories
		 WHERE search @@ to_tsquery('simple', $1) AND user_id = $2
		 ORDER BY ts_rank(search, to_tsquery('simple', $1)) DESC
		 LIMIT $3`, tsq, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// memoryTsquery turns arbitrary recall text into a broad OR tsquery for the
// 'simple' text-search config: lowercase alphanumeric tokens (len >= 3),
// deduped and capped at 12, joined with the tsquery OR operator. This mirrors
// the recall breadth the removed SQLite FTS5 backend provided. Returns "" when
// no usable terms remain, which callers treat as "no match".
func memoryTsquery(text string) string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	seen := make(map[string]bool)
	terms := make([]string, 0, 12)
	for _, f := range fields {
		if len([]rune(f)) < 3 || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, f)
		if len(terms) >= 12 {
			break
		}
	}
	return strings.Join(terms, " | ")
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
