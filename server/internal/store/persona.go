package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// defaultPersona is used when a user has no saved persona.
func defaultPersona() UserPersona {
	return UserPersona{Tone: "balanced", Emoji: "occasional", Length: "balanced", Personality: "balanced"}
}

func (s *SQLiteStore) GetUserPersona(ctx context.Context, userID int64) (UserPersona, error) {
	p := defaultPersona()
	err := s.db.QueryRowContext(ctx,
		`SELECT tone, emoji, length, personality, name, custom FROM user_personas WHERE user_id = ?`, userID,
	).Scan(&p.Tone, &p.Emoji, &p.Length, &p.Personality, &p.Name, &p.Custom)
	if err == sql.ErrNoRows {
		return defaultPersona(), nil
	}
	if err != nil {
		return defaultPersona(), fmt.Errorf("get persona: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) SetUserPersona(ctx context.Context, userID int64, p UserPersona) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_personas (user_id, tone, emoji, length, personality, name, custom, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   tone = excluded.tone, emoji = excluded.emoji, length = excluded.length,
		   personality = excluded.personality, name = excluded.name, custom = excluded.custom,
		   updated_at = excluded.updated_at`,
		userID, p.Tone, p.Emoji, p.Length, p.Personality, p.Name, p.Custom, time.Now().UTC())
	return err
}
