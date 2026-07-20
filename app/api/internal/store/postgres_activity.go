package store

import "context"

// countActiveReminders and countUserNotes back the Postgres half of
// HybridStore.GetUserActivity — the cross-backend method that also needs
// trace-derived run/token totals from MongoStore. They mirror the reminder/notes
// counts in the SQLite GetUserActivity.

func (s *PostgresStore) countActiveReminders(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reminders WHERE user_id = $1 AND notified = false AND cancelled = false`,
		userID,
	).Scan(&n)
	return n, err
}

func (s *PostgresStore) countUserNotes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notes WHERE user_id = $1`, userID,
	).Scan(&n)
	return n, err
}
