package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateNote(ctx context.Context, userID int64, title, content, tags string) (*Note, error) {
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO notes (user_id, project_id, title, content, tags, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		userID, authctx.ProjectID(ctx), title, content, tags, now, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}
	return &Note{
		ID:        id,
		Title:     title,
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *PostgresStore) GetNote(ctx context.Context, userID, id int64) (*Note, error) {
	var n Note
	err := s.pool.QueryRow(ctx,
		`SELECT id, title, content, tags, created_at, updated_at FROM notes WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, id, userID, authctx.ProjectID(ctx),
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	return &n, nil
}

func (s *PostgresStore) UpdateNote(ctx context.Context, userID, id int64, title, content, tags string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE notes SET title = $1, content = $2, tags = $3, updated_at = $4 WHERE id = $5 AND user_id = $6 AND ($7 = 0 OR project_id = $7)`,
		title, content, tags, time.Now().UTC(), id, userID, authctx.ProjectID(ctx),
	)
	return err
}

func (s *PostgresStore) DeleteNote(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, id, userID, authctx.ProjectID(ctx))
	return err
}

func (s *PostgresStore) ListNotes(ctx context.Context, userID int64, tag string) ([]Note, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if tag != "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes
			 WHERE user_id = $1 AND ',' || tags || ',' LIKE '%,' || $2 || ',%' AND ($3 = 0 OR project_id = $3)
			 ORDER BY updated_at DESC`, userID, tag, authctx.ProjectID(ctx),
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, title, content, tags, created_at, updated_at FROM notes WHERE user_id = $1 AND ($2 = 0 OR project_id = $2) ORDER BY updated_at DESC`,
			userID, authctx.ProjectID(ctx),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	return pgScanNotes(rows)
}

// SearchNotes returns the user's notes matching arbitrary query text, best-match
// first. The raw text is handed to Postgres' websearch_to_tsquery on the
// generated tsvector column; an empty or whitespace-only query returns no results.
func (s *PostgresStore) SearchNotes(ctx context.Context, userID int64, query string) ([]Note, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, content, tags, created_at, updated_at FROM notes
		 WHERE search @@ websearch_to_tsquery('simple', $1) AND user_id = $2 AND ($3 = 0 OR project_id = $3)
		 ORDER BY ts_rank(search, websearch_to_tsquery('simple', $1)) DESC`, query, userID, authctx.ProjectID(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer rows.Close()

	return pgScanNotes(rows)
}

// pgScanNotes scans note rows from a pgx.Rows. It is the pgx-typed counterpart of
// the SQLite backend's scanNotes (which takes *sql.Rows).
func pgScanNotes(rows pgx.Rows) ([]Note, error) {
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
