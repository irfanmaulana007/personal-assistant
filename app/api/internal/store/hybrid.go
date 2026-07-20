package store

import (
	"context"
	"errors"
	"fmt"
)

// HybridStore is the split backend: main data served by PostgreSQL, logs by
// MongoDB. It embeds both concrete stores, so Go method promotion supplies the
// whole DataStore half (from *PostgresStore) and the whole LogStore half (from
// *MongoStore) for free. Only two methods are written by hand:
//
//   - Close: both embedded stores define Close, so the promoted method is
//     ambiguous and must be resolved here (closing both).
//   - GetUserActivity: it spans both backends (trace totals from Mongo, reminder
//     and note counts from Postgres) and belongs to neither sub-interface.
//
// SetTranslator is promoted from *PostgresStore (a DataStore concern); logs need
// no translation.
var _ Store = (*HybridStore)(nil)

type HybridStore struct {
	*PostgresStore
	*MongoStore
}

// NewHybrid composes a PostgreSQL data store and a MongoDB log store into a
// single Store.
func NewHybrid(pg *PostgresStore, mongo *MongoStore) *HybridStore {
	return &HybridStore{PostgresStore: pg, MongoStore: mongo}
}

// Close shuts down both backends, joining any errors.
func (h *HybridStore) Close() error {
	return errors.Join(h.PostgresStore.Close(), h.MongoStore.Close())
}

// GetUserActivity aggregates the user's trace-derived usage (runs + tokens, from
// MongoDB) with their active reminders and notes (from PostgreSQL). This mirrors
// the single-backend SQLite GetUserActivity, fanned out across the two stores.
func (h *HybridStore) GetUserActivity(ctx context.Context, userID int64) (*UserActivity, error) {
	runs, totalTokens, err := h.MongoStore.userRunTotals(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user runs: %w", err)
	}
	reminders, err := h.PostgresStore.countActiveReminders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user reminders: %w", err)
	}
	notes, err := h.PostgresStore.countUserNotes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user notes: %w", err)
	}
	return &UserActivity{
		Runs:        runs,
		TotalTokens: totalTokens,
		Reminders:   reminders,
		Notes:       notes,
	}, nil
}
