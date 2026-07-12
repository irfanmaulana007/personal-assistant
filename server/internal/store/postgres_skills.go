package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// seedSkills upserts the code-owned master list of skills and prunes any
// retired ones (and their per-user toggles). It reuses the same skillSeed and
// prunedSkillKeys package-level definitions as the SQLite backend, and is
// idempotent via ON CONFLICT (key) DO UPDATE. NewPostgres wires the call.
func (s *PostgresStore) seedSkills(ctx context.Context) error {
	// Remove skills that were retired from the product (and their per-user toggles).
	for _, key := range prunedSkillKeys {
		var id int64
		err := s.pool.QueryRow(ctx, `SELECT id FROM skills WHERE key = $1`, key).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("prune lookup %s: %w", key, err)
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM user_skills WHERE skill_id = $1`, id); err != nil {
			return fmt.Errorf("prune user_skills %s: %w", key, err)
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id); err != nil {
			return fmt.Errorf("prune skill %s: %w", key, err)
		}
	}
	for _, sk := range skillSeed {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO skills (key, name, description, prompt, category, default_enabled, sort_order)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (key) DO UPDATE SET
			   name = EXCLUDED.name,
			   description = EXCLUDED.description,
			   prompt = EXCLUDED.prompt,
			   category = EXCLUDED.category,
			   default_enabled = EXCLUDED.default_enabled,
			   sort_order = EXCLUDED.sort_order`,
			sk.Key, sk.Name, sk.Description, sk.Prompt, sk.Category, sk.DefaultEnabled, sk.SortOrder,
		); err != nil {
			return fmt.Errorf("seed skill %s: %w", sk.Key, err)
		}
	}
	return nil
}

// --- Skills ---

func (s *PostgresStore) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, key, name, description, prompt, tuned_prompt, category, default_enabled, sort_order
		 FROM skills ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var out []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.TunedPrompt, &sk.Category, &sk.DefaultEnabled, &sk.SortOrder); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetSkill(ctx context.Context, id int64) (*Skill, error) {
	var sk Skill
	err := s.pool.QueryRow(ctx,
		`SELECT id, key, name, description, prompt, tuned_prompt, category, default_enabled, sort_order FROM skills WHERE id = $1`, id,
	).Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.TunedPrompt, &sk.Category, &sk.DefaultEnabled, &sk.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return &sk, nil
}

// ListUserSkills returns all skills with the effective enabled state for a user
// (the user's override if present, otherwise the skill's default).
func (s *PostgresStore) ListUserSkills(ctx context.Context, userID int64) ([]UserSkill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.key, s.name, s.description, s.prompt, s.tuned_prompt, s.category, s.default_enabled, s.sort_order,
		        COALESCE(us.enabled, s.default_enabled) AS effective
		 FROM skills s
		 LEFT JOIN user_skills us ON us.skill_id = s.id AND us.user_id = $1
		 ORDER BY s.sort_order ASC, s.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user skills: %w", err)
	}
	defer rows.Close()

	var out []UserSkill
	for rows.Next() {
		var us UserSkill
		if err := rows.Scan(&us.ID, &us.Key, &us.Name, &us.Description, &us.Prompt, &us.TunedPrompt, &us.Category, &us.DefaultEnabled, &us.SortOrder, &us.Enabled); err != nil {
			return nil, fmt.Errorf("scan user skill: %w", err)
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SetSkillEnabled(ctx context.Context, userID, skillID int64, enabled bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_skills (user_id, skill_id, enabled) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, skill_id) DO UPDATE SET enabled = EXCLUDED.enabled`,
		userID, skillID, enabled,
	)
	return err
}

// UpdateSkillTunedPrompt sets a skill's auto-tuned prompt override (or clears it
// with an empty string). It never touches `prompt`, so the shipped default is
// preserved as the reset target.
func (s *PostgresStore) UpdateSkillTunedPrompt(ctx context.Context, key, tuned string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE skills SET tuned_prompt = $2 WHERE key = $1`, key, tuned)
	if err != nil {
		return fmt.Errorf("update skill tuned prompt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("skill %q not found", key)
	}
	return nil
}

func (s *PostgresStore) EnabledSkillKeys(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.key FROM skills s
		 LEFT JOIN user_skills us ON us.skill_id = s.id AND us.user_id = $1
		 WHERE COALESCE(us.enabled, s.default_enabled) = true`, userID)
	if err != nil {
		return nil, fmt.Errorf("enabled skill keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
