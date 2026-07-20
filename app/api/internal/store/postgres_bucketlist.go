package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateBucketItem(ctx context.Context, userID int64, title, description, note, category string, resolutionYear *int) (*BucketItem, error) {
	title = s.enTitle(ctx, title)
	description = s.enText(ctx, description)
	note = s.enText(ctx, note)
	category = NormalizeCategory(category)
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO bucket_list_items (user_id, project_id, title, description, note, category, resolution_year, done, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8) RETURNING id`,
		userID, authctx.ProjectID(ctx), title, description, note, category, resolutionYear, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert bucket item: %w", err)
	}
	return &BucketItem{ID: id, Title: title, Description: description, Note: note, Category: category, ResolutionYear: resolutionYear, Done: false, CreatedAt: now}, nil
}

// ListBucketItems returns the user's items in the active project, unfinished
// first, newest within a group.
func (s *PostgresStore) ListBucketItems(ctx context.Context, userID int64) ([]BucketItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+bucketItemCols+` FROM bucket_list_items WHERE user_id = $1 AND ($2 = 0 OR project_id = $2)
		 ORDER BY done ASC, created_at DESC, id DESC`, userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list bucket items: %w", err)
	}
	defer rows.Close()
	return pgScanBucketItems(rows)
}

func (s *PostgresStore) GetBucketItem(ctx context.Context, userID, id int64) (*BucketItem, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+bucketItemCols+` FROM bucket_list_items WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`,
		id, userID, authctx.ProjectID(ctx))
	g, err := pgScanBucketItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get bucket item: %w", err)
	}
	return g, nil
}

func (s *PostgresStore) UpdateBucketItem(ctx context.Context, userID, id int64, title, description, note, category string) error {
	title = s.enTitle(ctx, title)
	description = s.enText(ctx, description)
	note = s.enText(ctx, note)
	category = NormalizeCategory(category)
	_, err := s.pool.Exec(ctx,
		`UPDATE bucket_list_items SET title = $1, description = $2, note = $3, category = $4 WHERE id = $5 AND user_id = $6 AND ($7 = 0 OR project_id = $7)`,
		title, description, note, category, id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("update bucket item: %w", err)
	}
	return nil
}

// SetBucketItemDone checks/unchecks an item, stamping done_at when checked.
// When done and doneAt is non-nil, that timestamp is recorded; otherwise it
// falls back to the current time. done_at is cleared when unchecked.
func (s *PostgresStore) SetBucketItemDone(ctx context.Context, userID, id int64, done bool, doneAt *time.Time) error {
	var at any
	if done {
		if doneAt != nil {
			at = doneAt.UTC()
		} else {
			at = time.Now().UTC()
		}
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE bucket_list_items SET done = $1, done_at = $2 WHERE id = $3 AND user_id = $4 AND ($5 = 0 OR project_id = $5)`,
		done, at, id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("set bucket item done: %w", err)
	}
	return nil
}

// SetBucketItemResolution flags an item as a resolution for the given year, or
// clears the flag when year is nil.
func (s *PostgresStore) SetBucketItemResolution(ctx context.Context, userID, id int64, year *int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE bucket_list_items SET resolution_year = $1 WHERE id = $2 AND user_id = $3 AND ($4 = 0 OR project_id = $4)`,
		year, id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("set bucket item resolution: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteBucketItem(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM bucket_list_items WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`,
		id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("delete bucket item: %w", err)
	}
	return nil
}

// pgRowScanner is satisfied by both pgx.Row and pgx.Rows.
type pgRowScanner interface {
	Scan(dest ...any) error
}

func pgScanBucketItem(sc pgRowScanner) (*BucketItem, error) {
	var g BucketItem
	var doneAt *time.Time
	var resYear *int
	// Column order matches bucketItemCols: id, title, description, note, category, resolution_year, done, created_at, done_at.
	if err := sc.Scan(&g.ID, &g.Title, &g.Description, &g.Note, &g.Category, &resYear, &g.Done, &g.CreatedAt, &doneAt); err != nil {
		return nil, err
	}
	g.ResolutionYear = resYear
	if doneAt != nil {
		g.DoneAt = doneAt
	}
	return &g, nil
}

func pgScanBucketItems(rows pgx.Rows) ([]BucketItem, error) {
	var out []BucketItem
	for rows.Next() {
		g, err := pgScanBucketItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bucket item: %w", err)
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}
