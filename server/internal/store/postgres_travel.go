package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateTrip starts a new trip and makes it the user's active trip (any
// previously active trips are deactivated).
func (s *PostgresStore) CreateTrip(ctx context.Context, userID int64, name, destination, currency string, budget float64) (*Trip, error) {
	if _, err := s.pool.Exec(ctx, `UPDATE trips SET active = false WHERE user_id = $1`, userID); err != nil {
		return nil, fmt.Errorf("deactivate trips: %w", err)
	}
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trips (user_id, name, destination, budget, currency, active, started_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6) RETURNING id`,
		userID, name, destination, budget, currency, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert trip: %w", err)
	}
	return &Trip{ID: id, Name: name, Destination: destination, Budget: budget, Currency: currency, Active: true, StartedAt: now}, nil
}

// pgScanTrip scans a single trip row, returning (nil, nil) when no row matched
// (mirroring the SQLite backend's no-rows behavior).
func (s *PostgresStore) pgScanTrip(row pgx.Row) (*Trip, error) {
	var t Trip
	err := row.Scan(&t.ID, &t.Name, &t.Destination, &t.Budget, &t.Currency, &t.Active, &t.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan trip: %w", err)
	}
	return &t, nil
}

// ActiveTrip returns the user's current active trip, or nil if none.
func (s *PostgresStore) ActiveTrip(ctx context.Context, userID int64) (*Trip, error) {
	return s.pgScanTrip(s.pool.QueryRow(ctx,
		`SELECT id, name, destination, budget, currency, active, started_at FROM trips
		 WHERE user_id = $1 AND active = true ORDER BY started_at DESC LIMIT 1`, userID))
}

// FindTrip returns the user's most recent trip matching name (case-insensitive), or nil.
func (s *PostgresStore) FindTrip(ctx context.Context, userID int64, name string) (*Trip, error) {
	return s.pgScanTrip(s.pool.QueryRow(ctx,
		`SELECT id, name, destination, budget, currency, active, started_at FROM trips
		 WHERE user_id = $1 AND name ILIKE $2 ORDER BY started_at DESC LIMIT 1`, userID, "%"+name+"%"))
}

func (s *PostgresStore) AddExpense(ctx context.Context, userID, tripID int64, amount float64, currency, category, note string, spentAt time.Time) (*TripExpense, error) {
	if category == "" {
		category = "other"
	}
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trip_expenses (user_id, trip_id, amount, currency, category, note, spent_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		userID, tripID, amount, currency, category, note, spentAt.UTC(),
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert expense: %w", err)
	}
	return &TripExpense{ID: id, TripID: tripID, Amount: amount, Currency: currency, Category: category, Note: note, SpentAt: spentAt}, nil
}

func (s *PostgresStore) ListTripExpenses(ctx context.Context, userID, tripID int64) ([]TripExpense, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, trip_id, amount, currency, category, note, spent_at FROM trip_expenses
		 WHERE user_id = $1 AND trip_id = $2 ORDER BY spent_at DESC`, userID, tripID)
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
