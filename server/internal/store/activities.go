package store

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateActivity(ctx context.Context, userID int64, actType, description string, occurredAt time.Time, source string) (*Activity, error) {
	now := time.Now().UTC()
	if source == "" {
		source = "chat"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO activities (user_id, type, description, occurred_at, source, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, actType, description, occurredAt.UTC(), source, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Activity{ID: id, Type: actType, Description: description, OccurredAt: occurredAt, Source: source, CreatedAt: now}, nil
}

// ListActivitiesSince returns the user's activities on or after since, newest first.
func (s *SQLiteStore) ListActivitiesSince(ctx context.Context, userID int64, since time.Time) ([]Activity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, description, occurred_at, source, created_at
		 FROM activities WHERE user_id = ? AND occurred_at >= ?
		 ORDER BY occurred_at DESC`, userID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Type, &a.Description, &a.OccurredAt, &a.Source, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
