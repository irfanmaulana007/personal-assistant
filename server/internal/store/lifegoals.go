package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const lifeGoalCols = "id, title, description, note, done, created_at, done_at"

func (s *SQLiteStore) CreateLifeGoal(ctx context.Context, userID int64, title, description, note string) (*LifeGoal, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO life_goals (user_id, title, description, note, done, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
		userID, title, description, note, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert life goal: %w", err)
	}
	id, _ := res.LastInsertId()
	return &LifeGoal{ID: id, Title: title, Description: description, Note: note, Done: false, CreatedAt: now}, nil
}

// ListLifeGoals returns the user's goals, unfinished first, newest within a group.
func (s *SQLiteStore) ListLifeGoals(ctx context.Context, userID int64) ([]LifeGoal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+lifeGoalCols+` FROM life_goals WHERE user_id = ?
		 ORDER BY done ASC, created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list life goals: %w", err)
	}
	defer rows.Close()
	return scanLifeGoals(rows)
}

func (s *SQLiteStore) GetLifeGoal(ctx context.Context, userID, id int64) (*LifeGoal, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+lifeGoalCols+` FROM life_goals WHERE id = ? AND user_id = ?`, id, userID)
	g, err := scanLifeGoal(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get life goal: %w", err)
	}
	return g, nil
}

func (s *SQLiteStore) UpdateLifeGoal(ctx context.Context, userID, id int64, title, description, note string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE life_goals SET title = ?, description = ?, note = ? WHERE id = ? AND user_id = ?`,
		title, description, note, id, userID)
	if err != nil {
		return fmt.Errorf("update life goal: %w", err)
	}
	return nil
}

// SetLifeGoalDone checks/unchecks a goal, stamping done_at when checked.
func (s *SQLiteStore) SetLifeGoalDone(ctx context.Context, userID, id int64, done bool) error {
	var doneAt any
	if done {
		doneAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE life_goals SET done = ?, done_at = ? WHERE id = ? AND user_id = ?`,
		boolToInt(done), doneAt, id, userID)
	if err != nil {
		return fmt.Errorf("set life goal done: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteLifeGoal(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM life_goals WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete life goal: %w", err)
	}
	return nil
}

func scanLifeGoal(sc rowScanner) (*LifeGoal, error) {
	var g LifeGoal
	var done int
	var doneAt sql.NullTime
	if err := sc.Scan(&g.ID, &g.Title, &g.Description, &g.Note, &done, &g.CreatedAt, &doneAt); err != nil {
		return nil, err
	}
	g.Done = done != 0
	if doneAt.Valid {
		t := doneAt.Time
		g.DoneAt = &t
	}
	return &g, nil
}

func scanLifeGoals(rows *sql.Rows) ([]LifeGoal, error) {
	var out []LifeGoal
	for rows.Next() {
		g, err := scanLifeGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan life goal: %w", err)
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}
