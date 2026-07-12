package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const bucketItemCols = "id, title, description, note, category, resolution_year, done, created_at, done_at"

func (s *SQLiteStore) CreateBucketItem(ctx context.Context, userID int64, title, description, note, category string, resolutionYear *int) (*BucketItem, error) {
	title = s.enTitle(ctx, title)
	description = s.enText(ctx, description)
	note = s.enText(ctx, note)
	category = NormalizeCategory(category)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bucket_list_items (user_id, title, description, note, category, resolution_year, done, created_at) VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		userID, title, description, note, category, resolutionYear, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert bucket item: %w", err)
	}
	id, _ := res.LastInsertId()
	return &BucketItem{ID: id, Title: title, Description: description, Note: note, Category: category, ResolutionYear: resolutionYear, Done: false, CreatedAt: now}, nil
}

// ListBucketItems returns the user's items, unfinished first, newest within a group.
func (s *SQLiteStore) ListBucketItems(ctx context.Context, userID int64) ([]BucketItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+bucketItemCols+` FROM bucket_list_items WHERE user_id = ?
		 ORDER BY done ASC, created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list bucket items: %w", err)
	}
	defer rows.Close()
	return scanBucketItems(rows)
}

func (s *SQLiteStore) GetBucketItem(ctx context.Context, userID, id int64) (*BucketItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+bucketItemCols+` FROM bucket_list_items WHERE id = ? AND user_id = ?`, id, userID)
	g, err := scanBucketItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get bucket item: %w", err)
	}
	return g, nil
}

func (s *SQLiteStore) UpdateBucketItem(ctx context.Context, userID, id int64, title, description, note, category string) error {
	title = s.enTitle(ctx, title)
	description = s.enText(ctx, description)
	note = s.enText(ctx, note)
	category = NormalizeCategory(category)
	_, err := s.db.ExecContext(ctx,
		`UPDATE bucket_list_items SET title = ?, description = ?, note = ?, category = ? WHERE id = ? AND user_id = ?`,
		title, description, note, category, id, userID)
	if err != nil {
		return fmt.Errorf("update bucket item: %w", err)
	}
	return nil
}

// SetBucketItemDone checks/unchecks an item, stamping done_at when checked.
func (s *SQLiteStore) SetBucketItemDone(ctx context.Context, userID, id int64, done bool) error {
	var doneAt any
	if done {
		doneAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE bucket_list_items SET done = ?, done_at = ? WHERE id = ? AND user_id = ?`,
		boolToInt(done), doneAt, id, userID)
	if err != nil {
		return fmt.Errorf("set bucket item done: %w", err)
	}
	return nil
}

// SetBucketItemResolution flags an item as a resolution for the given year, or
// clears the flag when year is nil.
func (s *SQLiteStore) SetBucketItemResolution(ctx context.Context, userID, id int64, year *int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bucket_list_items SET resolution_year = ? WHERE id = ? AND user_id = ?`,
		year, id, userID)
	if err != nil {
		return fmt.Errorf("set bucket item resolution: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteBucketItem(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bucket_list_items WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete bucket item: %w", err)
	}
	return nil
}

func scanBucketItem(sc rowScanner) (*BucketItem, error) {
	var g BucketItem
	var done int
	var doneAt sql.NullTime
	var resYear sql.NullInt64
	if err := sc.Scan(&g.ID, &g.Title, &g.Description, &g.Note, &g.Category, &resYear, &done, &g.CreatedAt, &doneAt); err != nil {
		return nil, err
	}
	g.Done = done != 0
	if resYear.Valid {
		y := int(resYear.Int64)
		g.ResolutionYear = &y
	}
	if doneAt.Valid {
		t := doneAt.Time
		g.DoneAt = &t
	}
	return &g, nil
}

func scanBucketItems(rows *sql.Rows) ([]BucketItem, error) {
	var out []BucketItem
	for rows.Next() {
		g, err := scanBucketItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bucket item: %w", err)
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}
