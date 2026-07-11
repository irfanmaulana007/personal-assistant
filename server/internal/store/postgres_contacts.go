package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateContact(ctx context.Context, userID int64, name, phone, email, note string) (*Contact, error) {
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO contacts (user_id, name, phone, email, note, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, name, phone, email, note, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert contact: %w", err)
	}
	return &Contact{ID: id, Name: name, Phone: phone, Email: email, Note: note, CreatedAt: now}, nil
}

// SearchContacts returns the user's contacts matching query across name, phone,
// email, and note. An empty query returns all of the user's contacts. The SQLite
// source used LIKE, which is case-insensitive for ASCII; Postgres LIKE is
// case-sensitive, so ILIKE is used to preserve the original matching semantics.
func (s *PostgresStore) SearchContacts(ctx context.Context, userID int64, query string) ([]Contact, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if query == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, name, phone, email, note, created_at FROM contacts
			 WHERE user_id = $1 ORDER BY name ASC`, userID)
	} else {
		like := "%" + query + "%"
		rows, err = s.pool.Query(ctx,
			`SELECT id, name, phone, email, note, created_at FROM contacts
			 WHERE user_id = $1 AND (name ILIKE $2 OR phone ILIKE $2 OR email ILIKE $2 OR note ILIKE $2)
			 ORDER BY name ASC`, userID, like)
	}
	if err != nil {
		return nil, fmt.Errorf("search contacts: %w", err)
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Note, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
