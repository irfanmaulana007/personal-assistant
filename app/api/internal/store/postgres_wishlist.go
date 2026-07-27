package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateWishItem(ctx context.Context, userID int64, name string, estimatedPrice int64, buyMonth, priority, link, note string) (*WishItem, error) {
	name = s.enTitle(ctx, name)
	note = s.enText(ctx, note)
	priority = NormalizeWishPriority(priority)
	if estimatedPrice < 0 {
		estimatedPrice = 0
	}
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO wishlist_items (user_id, project_id, name, estimated_price, buy_month, priority, link, note, done, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, $9) RETURNING id`,
		userID, authctx.ProjectID(ctx), name, estimatedPrice, buyMonth, priority, link, note, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert wish item: %w", err)
	}
	return &WishItem{ID: id, Name: name, EstimatedPrice: estimatedPrice, BuyMonth: buyMonth, Priority: priority, Link: link, Note: note, Done: false, CreatedAt: now}, nil
}

// ListWishItems returns the user's items in the active project, unfinished
// first, then dated items by target month, undated last, newest within a group.
func (s *PostgresStore) ListWishItems(ctx context.Context, userID int64) ([]WishItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+wishItemCols+` FROM wishlist_items WHERE user_id = $1 AND ($2 = 0 OR project_id = $2)
		 ORDER BY done ASC, (buy_month = '') ASC, buy_month ASC, created_at DESC, id DESC`,
		userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list wish items: %w", err)
	}
	defer rows.Close()
	return pgScanWishItems(rows)
}

func (s *PostgresStore) GetWishItem(ctx context.Context, userID, id int64) (*WishItem, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+wishItemCols+` FROM wishlist_items WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`,
		id, userID, authctx.ProjectID(ctx))
	w, err := pgScanWishItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get wish item: %w", err)
	}
	return w, nil
}

func (s *PostgresStore) UpdateWishItem(ctx context.Context, userID, id int64, name string, estimatedPrice int64, buyMonth, priority, link, note string) error {
	name = s.enTitle(ctx, name)
	note = s.enText(ctx, note)
	priority = NormalizeWishPriority(priority)
	if estimatedPrice < 0 {
		estimatedPrice = 0
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE wishlist_items SET name = $1, estimated_price = $2, buy_month = $3, priority = $4, link = $5, note = $6
		 WHERE id = $7 AND user_id = $8 AND ($9 = 0 OR project_id = $9)`,
		name, estimatedPrice, buyMonth, priority, link, note, id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("update wish item: %w", err)
	}
	return nil
}

// SetWishItemDone marks an item as purchased/checked out (or reopens it),
// stamping done_at when checked. When done and doneAt is non-nil, that timestamp
// is recorded; otherwise it falls back to now. done_at is cleared when unchecked.
func (s *PostgresStore) SetWishItemDone(ctx context.Context, userID, id int64, done bool, doneAt *time.Time) error {
	var at any
	if done {
		if doneAt != nil {
			at = doneAt.UTC()
		} else {
			at = time.Now().UTC()
		}
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE wishlist_items SET done = $1, done_at = $2 WHERE id = $3 AND user_id = $4 AND ($5 = 0 OR project_id = $5)`,
		done, at, id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("set wish item done: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteWishItem(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM wishlist_items WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`,
		id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("delete wish item: %w", err)
	}
	return nil
}

func pgScanWishItem(sc pgRowScanner) (*WishItem, error) {
	var w WishItem
	var doneAt *time.Time
	// Column order matches wishItemCols: id, name, estimated_price, buy_month, priority, link, note, done, created_at, done_at.
	if err := sc.Scan(&w.ID, &w.Name, &w.EstimatedPrice, &w.BuyMonth, &w.Priority, &w.Link, &w.Note, &w.Done, &w.CreatedAt, &doneAt); err != nil {
		return nil, err
	}
	if doneAt != nil {
		w.DoneAt = doneAt
	}
	return &w, nil
}

func pgScanWishItems(rows pgx.Rows) ([]WishItem, error) {
	var out []WishItem
	for rows.Next() {
		w, err := pgScanWishItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan wish item: %w", err)
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}
