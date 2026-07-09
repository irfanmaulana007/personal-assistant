package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateTrip starts a new trip and makes it the user's active trip (any
// previously active trips are deactivated).
func (s *SQLiteStore) CreateTrip(ctx context.Context, userID int64, name, destination, currency string, budget float64) (*Trip, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE trips SET active = 0 WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("deactivate trips: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO trips (user_id, name, destination, budget, currency, active, started_at) VALUES (?, ?, ?, ?, ?, 1, ?)`,
		userID, name, destination, budget, currency, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert trip: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Trip{ID: id, Name: name, Destination: destination, Budget: budget, Currency: currency, Active: true, StartedAt: now}, nil
}

func (s *SQLiteStore) scanTrip(row *sql.Row) (*Trip, error) {
	var t Trip
	var active int
	err := row.Scan(&t.ID, &t.Name, &t.Destination, &t.Budget, &t.Currency, &active, &t.StartedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan trip: %w", err)
	}
	t.Active = active != 0
	return &t, nil
}

// ActiveTrip returns the user's current active trip, or nil if none.
func (s *SQLiteStore) ActiveTrip(ctx context.Context, userID int64) (*Trip, error) {
	return s.scanTrip(s.db.QueryRowContext(ctx,
		`SELECT id, name, destination, budget, currency, active, started_at FROM trips
		 WHERE user_id = ? AND active = 1 ORDER BY started_at DESC LIMIT 1`, userID))
}

// FindTrip returns the user's most recent trip matching name (case-insensitive), or nil.
func (s *SQLiteStore) FindTrip(ctx context.Context, userID int64, name string) (*Trip, error) {
	return s.scanTrip(s.db.QueryRowContext(ctx,
		`SELECT id, name, destination, budget, currency, active, started_at FROM trips
		 WHERE user_id = ? AND name LIKE ? ORDER BY started_at DESC LIMIT 1`, userID, "%"+name+"%"))
}

func (s *SQLiteStore) AddExpense(ctx context.Context, userID, tripID int64, amount float64, currency, category, note string, spentAt time.Time) (*TripExpense, error) {
	if category == "" {
		category = "other"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO trip_expenses (user_id, trip_id, amount, currency, category, note, spent_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, tripID, amount, currency, category, note, spentAt.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert expense: %w", err)
	}
	id, _ := res.LastInsertId()
	return &TripExpense{ID: id, TripID: tripID, Amount: amount, Currency: currency, Category: category, Note: note, SpentAt: spentAt}, nil
}

func (s *SQLiteStore) ListTripExpenses(ctx context.Context, userID, tripID int64) ([]TripExpense, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, trip_id, amount, currency, category, note, spent_at FROM trip_expenses
		 WHERE user_id = ? AND trip_id = ? ORDER BY spent_at DESC`, userID, tripID)
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()

	var out []TripExpense
	for rows.Next() {
		var e TripExpense
		if err := rows.Scan(&e.ID, &e.TripID, &e.Amount, &e.Currency, &e.Category, &e.Note, &e.SpentAt); err != nil {
			return nil, fmt.Errorf("scan expense: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
