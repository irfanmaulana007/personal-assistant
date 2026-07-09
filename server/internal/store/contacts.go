package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateContact(ctx context.Context, userID int64, name, phone, email, note string) (*Contact, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO contacts (user_id, name, phone, email, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, name, phone, email, note, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert contact: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Contact{ID: id, Name: name, Phone: phone, Email: email, Note: note, CreatedAt: now}, nil
}

// SearchContacts returns the user's contacts matching query across name, phone,
// email, and note. An empty query returns all of the user's contacts.
func (s *SQLiteStore) SearchContacts(ctx context.Context, userID int64, query string) ([]Contact, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if query == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, phone, email, note, created_at FROM contacts WHERE user_id = ? ORDER BY name ASC`, userID)
	} else {
		like := "%" + query + "%"
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, phone, email, note, created_at FROM contacts
			 WHERE user_id = ? AND (name LIKE ? OR phone LIKE ? OR email LIKE ? OR note LIKE ?)
			 ORDER BY name ASC`, userID, like, like, like, like)
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
