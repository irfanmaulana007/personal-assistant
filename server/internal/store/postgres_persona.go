package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) GetUserPersona(ctx context.Context, userID int64) (UserPersona, error) {
	p := defaultPersona()
	err := s.pool.QueryRow(ctx,
		`SELECT tone, emoji, length, personality, name, custom FROM user_personas WHERE user_id = $1`, userID,
	).Scan(&p.Tone, &p.Emoji, &p.Length, &p.Personality, &p.Name, &p.Custom)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultPersona(), nil
	}
	if err != nil {
		return defaultPersona(), fmt.Errorf("get persona: %w", err)
	}
	return p, nil
}

func (s *PostgresStore) SetUserPersona(ctx context.Context, userID int64, p UserPersona) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_personas (user_id, tone, emoji, length, personality, name, custom, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id) DO UPDATE SET
		   tone = EXCLUDED.tone, emoji = EXCLUDED.emoji, length = EXCLUDED.length,
		   personality = EXCLUDED.personality, name = EXCLUDED.name, custom = EXCLUDED.custom,
		   updated_at = EXCLUDED.updated_at`,
		userID, p.Tone, p.Emoji, p.Length, p.Personality, p.Name, p.Custom, time.Now().UTC())
	return err
}
