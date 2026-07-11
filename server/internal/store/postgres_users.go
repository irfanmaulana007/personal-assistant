package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Users ---

func (s *PostgresStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *PostgresStore) CreateUser(ctx context.Context, email, passwordHash, role string) (*User, error) {
	now := time.Now().UTC()
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role, created_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		email, passwordHash, role, now,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return &User{ID: id, Email: email, PasswordHash: passwordHash, Role: role, CreatedAt: now}, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return pgScanUser(s.pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE email = $1`, email))
}

func (s *PostgresStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return pgScanUser(s.pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE id = $1`, id))
}

func (s *PostgresStore) FirstAdmin(ctx context.Context) (*User, error) {
	return pgScanUser(s.pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1`))
}

func pgScanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func (s *PostgresStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *PostgresStore) UpdateUserRole(ctx context.Context, id int64, role string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET role = $1 WHERE id = $2`, role, id)
	return err
}

func (s *PostgresStore) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, id)
	return err
}

func (s *PostgresStore) UpdateUserProfile(ctx context.Context, id int64, name, email string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET name = $1, email = $2 WHERE id = $3`, name, email, id)
	return err
}

func (s *PostgresStore) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}
