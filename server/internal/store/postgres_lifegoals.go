package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateLifeGoal(ctx context.Context, userID int64, title, note string) (*LifeGoal, error) {
	title = s.enTitle(ctx, title)
	note = s.enText(ctx, note)
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO life_goals (user_id, title, note, done, created_at)
		 VALUES ($1, $2, $3, false, $4) RETURNING id`,
		userID, title, note, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert life goal: %w", err)
	}
	return &LifeGoal{ID: id, Title: title, Note: note, Done: false, CreatedAt: now}, nil
}

// ListLifeGoals returns the user's goals, unfinished first, newest within a group.
func (s *PostgresStore) ListLifeGoals(ctx context.Context, userID int64) ([]LifeGoal, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+lifeGoalCols+` FROM life_goals WHERE user_id = $1
		 ORDER BY done ASC, created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list life goals: %w", err)
	}
	defer rows.Close()
	return pgScanLifeGoals(rows)
}

func (s *PostgresStore) GetLifeGoal(ctx context.Context, userID, id int64) (*LifeGoal, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+lifeGoalCols+` FROM life_goals WHERE id = $1 AND user_id = $2`, id, userID)
	g, err := pgScanLifeGoal(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get life goal: %w", err)
	}
	return g, nil
}

func (s *PostgresStore) UpdateLifeGoal(ctx context.Context, userID, id int64, title, note string) error {
	title = s.enTitle(ctx, title)
	note = s.enText(ctx, note)
	_, err := s.pool.Exec(ctx,
		`UPDATE life_goals SET title = $1, note = $2 WHERE id = $3 AND user_id = $4`,
		title, note, id, userID)
	if err != nil {
		return fmt.Errorf("update life goal: %w", err)
	}
	return nil
}

// SetLifeGoalDone checks/unchecks a goal, stamping done_at when checked.
func (s *PostgresStore) SetLifeGoalDone(ctx context.Context, userID, id int64, done bool) error {
	var doneAt any
	if done {
		doneAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE life_goals SET done = $1, done_at = $2 WHERE id = $3 AND user_id = $4`,
		done, doneAt, id, userID)
	if err != nil {
		return fmt.Errorf("set life goal done: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteLifeGoal(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM life_goals WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete life goal: %w", err)
	}
	return nil
}

// pgRowScanner is satisfied by both pgx.Row and pgx.Rows.
type pgRowScanner interface {
	Scan(dest ...any) error
}

func pgScanLifeGoal(sc pgRowScanner) (*LifeGoal, error) {
	var g LifeGoal
	var doneAt *time.Time
	if err := sc.Scan(&g.ID, &g.Title, &g.Note, &g.Done, &g.CreatedAt, &doneAt); err != nil {
		return nil, err
	}
	if doneAt != nil {
		g.DoneAt = doneAt
	}
	return &g, nil
}

func pgScanLifeGoals(rows pgx.Rows) ([]LifeGoal, error) {
	var out []LifeGoal
	for rows.Next() {
		g, err := pgScanLifeGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan life goal: %w", err)
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}
