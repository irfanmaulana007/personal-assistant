package store

import (
	"context"
	"encoding/json"
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
	// Only the global (code-seeded) row is pruned — project forks are user data.
	for _, key := range prunedSkillKeys {
		var id int64
		err := s.pool.QueryRow(ctx, `SELECT id FROM skills WHERE key = $1 AND project_id IS NULL`, key).Scan(&id)
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
			`INSERT INTO skills (key, name, description, prompt, category, default_enabled, sort_order, is_core, project_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL)
			 ON CONFLICT (key) WHERE project_id IS NULL DO UPDATE SET
			   name = EXCLUDED.name,
			   description = EXCLUDED.description,
			   -- Preserve an admin-customized prompt; only re-seed the default
			   -- while the prompt has never been edited (prompt_updated_at IS NULL).
			   prompt = CASE WHEN skills.prompt_updated_at IS NULL THEN EXCLUDED.prompt ELSE skills.prompt END,
			   category = EXCLUDED.category,
			   default_enabled = EXCLUDED.default_enabled,
			   sort_order = EXCLUDED.sort_order`,
			// is_core is only applied on first insert — after that it is
			// superadmin-managed, so it is deliberately left out of DO UPDATE.
			sk.Key, sk.Name, sk.Description, sk.Prompt, sk.Category, sk.DefaultEnabled, sk.SortOrder, sk.IsCore,
		); err != nil {
			return fmt.Errorf("seed skill %s: %w", sk.Key, err)
		}
	}
	return nil
}

// --- Skills ---

// ListSkills returns the global (code-seeded) skill catalog only — project
// forks are excluded. It backs the boot seed, the self-tuner/auto-triage
// (which key skills by their stable global key), and the superadmin skill
// scope.
func (s *PostgresStore) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, key, name, description, prompt, tuned_prompt, category, default_enabled, sort_order, prompt_updated_at, prompt_updated_by, project_id, is_core
		 FROM skills WHERE project_id IS NULL ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var out []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.TunedPrompt, &sk.Category, &sk.DefaultEnabled, &sk.SortOrder, &sk.PromptUpdatedAt, &sk.PromptUpdatedBy, &sk.ProjectID, &sk.IsCore); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetSkill(ctx context.Context, id int64) (*Skill, error) {
	var sk Skill
	err := s.pool.QueryRow(ctx,
		`SELECT id, key, name, description, prompt, tuned_prompt, category, default_enabled, sort_order, prompt_updated_at, prompt_updated_by, project_id, is_core FROM skills WHERE id = $1`, id,
	).Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.TunedPrompt, &sk.Category, &sk.DefaultEnabled, &sk.SortOrder, &sk.PromptUpdatedAt, &sk.PromptUpdatedBy, &sk.ProjectID, &sk.IsCore)
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
		        s.prompt_updated_at, s.prompt_updated_by, s.project_id, s.is_core,
		        COALESCE(us.enabled, s.default_enabled) AS effective
		 FROM skills s
		 LEFT JOIN user_skills us ON us.skill_id = s.id AND us.user_id = $1
		 WHERE s.project_id IS NULL
		 ORDER BY s.sort_order ASC, s.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user skills: %w", err)
	}
	defer rows.Close()

	var out []UserSkill
	for rows.Next() {
		var us UserSkill
		if err := rows.Scan(&us.ID, &us.Key, &us.Name, &us.Description, &us.Prompt, &us.TunedPrompt, &us.Category, &us.DefaultEnabled, &us.SortOrder, &us.PromptUpdatedAt, &us.PromptUpdatedBy, &us.ProjectID, &us.IsCore, &us.Enabled); err != nil {
			return nil, fmt.Errorf("scan user skill: %w", err)
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

// SetSkillPrompt updates a skill's prompt. A non-empty updatedBy marks the
// change as an admin customization (protected from the boot seed); an empty
// updatedBy resets to the code default and lets the seed manage it again.
func (s *PostgresStore) SetSkillPrompt(ctx context.Context, skillID int64, prompt, updatedBy string) error {
	if updatedBy == "" {
		_, err := s.pool.Exec(ctx,
			`UPDATE skills SET prompt = $1, prompt_updated_at = NULL, prompt_updated_by = '' WHERE id = $2`,
			prompt, skillID,
		)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE skills SET prompt = $1, prompt_updated_at = now(), prompt_updated_by = $2 WHERE id = $3`,
		prompt, updatedBy, skillID,
	)
	return err
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
	// The self-tuner keys skills by their stable global key; scope to the global
	// row so a project fork sharing the key is never touched.
	tag, err := s.pool.Exec(ctx, `UPDATE skills SET tuned_prompt = $2 WHERE key = $1 AND project_id IS NULL`, key, tuned)
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

// CreateProjectSkill inserts a project-owned fork of a skill. The prompt is
// stamped as an admin customization (prompt_updated_at = now, prompt_updated_by
// = updatedBy) so it is treated as owned by the project. tuned_prompt is left
// empty — the self-tuner only touches global skills.
func (s *PostgresStore) CreateProjectSkill(ctx context.Context, projectID int64, base Skill, updatedBy string) (*Skill, error) {
	var sk Skill
	err := s.pool.QueryRow(ctx,
		`INSERT INTO skills (project_id, key, name, description, prompt, category, default_enabled, sort_order, prompt_updated_at, prompt_updated_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)
		 RETURNING id, key, name, description, prompt, tuned_prompt, category, default_enabled, sort_order, prompt_updated_at, prompt_updated_by, project_id, is_core`,
		projectID, base.Key, base.Name, base.Description, base.Prompt, base.Category, base.DefaultEnabled, base.SortOrder, updatedBy,
	).Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.TunedPrompt, &sk.Category, &sk.DefaultEnabled, &sk.SortOrder, &sk.PromptUpdatedAt, &sk.PromptUpdatedBy, &sk.ProjectID, &sk.IsCore)
	if err != nil {
		return nil, fmt.Errorf("create project skill: %w", err)
	}
	return &sk, nil
}

// DeleteProjectSkill removes a project's fork (and its per-project enable row),
// reverting that project to the shared global skill. It only deletes a skill
// that belongs to the given project, so a global skill can never be removed
// through this path.
func (s *PostgresStore) DeleteProjectSkill(ctx context.Context, projectID, skillID int64) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM project_skills WHERE project_id = $1 AND skill_id = $2`, projectID, skillID); err != nil {
		return fmt.Errorf("delete project skill toggle: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM skills WHERE id = $1 AND project_id = $2`, skillID, projectID)
	if err != nil {
		return fmt.Errorf("delete project skill: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project skill %d not found in project %d", skillID, projectID)
	}
	return nil
}

// SetSkillCore marks or unmarks a global skill as core. Only global skills
// (project_id IS NULL) can be core — project forks are always project-specific,
// so a fork id affects no rows and returns an error the caller can surface.
func (s *PostgresStore) SetSkillCore(ctx context.Context, skillID int64, isCore bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE skills SET is_core = $2 WHERE id = $1 AND project_id IS NULL`, skillID, isCore)
	if err != nil {
		return fmt.Errorf("set skill core: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("global skill %d not found", skillID)
	}
	return nil
}

// ListSkillsWithProjectMapping returns every skill (global skills and project
// forks) together with the projects that effectively enable it — the mapping
// the superadmin catalog uses to classify each skill as core / global /
// project-specific. A skill enabled in no project comes back with an empty
// project list. Effective-enabled reuses the same per-project cascade
// (per-project toggle AND feature gate) and scope rules as ListProjectSkills.
func (s *PostgresStore) ListSkillsWithProjectMapping(ctx context.Context) ([]SkillWithMapping, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.key, s.name, s.description, s.prompt, s.tuned_prompt, s.category,
		        s.default_enabled, s.sort_order, s.prompt_updated_at, s.prompt_updated_by, s.project_id, s.is_core,
		        COALESCE(
		          jsonb_agg(jsonb_build_object('id', p.id, 'name', p.name, 'slug', p.slug) ORDER BY p.name)
		            FILTER (WHERE p.id IS NOT NULL
		                    AND (COALESCE(ps.enabled, s.default_enabled) AND COALESCE(pf.enabled, f.default_enabled, true))),
		          '[]'::jsonb) AS projects
		 FROM skills s
		 -- A project is in scope for this skill when the skill is visible to it:
		 -- its own fork, or a global skill not shadowed by a fork of the same key.
		 LEFT JOIN projects p ON (
		         (s.project_id IS NULL OR s.project_id = p.id)
		     AND NOT (s.project_id IS NULL AND EXISTS (
		               SELECT 1 FROM skills o WHERE o.project_id = p.id AND o.key = s.key)))
		 LEFT JOIN project_skills ps ON ps.skill_id = s.id AND ps.project_id = p.id
		 -- Feature gate is keyed off the global twin (a fork inherits its feature).
		 LEFT JOIN skills gt ON gt.project_id IS NULL AND gt.key = s.key
		 LEFT JOIN feature_skills fs ON fs.skill_id = gt.id
		 LEFT JOIN features f ON f.id = fs.feature_id
		 LEFT JOIN project_features pf ON pf.feature_id = f.id AND pf.project_id = p.id
		 GROUP BY s.id
		 ORDER BY s.sort_order ASC, s.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list skills with project mapping: %w", err)
	}
	defer rows.Close()

	var out []SkillWithMapping
	for rows.Next() {
		var sm SkillWithMapping
		var projectsJSON []byte
		if err := rows.Scan(&sm.ID, &sm.Key, &sm.Name, &sm.Description, &sm.Prompt, &sm.TunedPrompt, &sm.Category,
			&sm.DefaultEnabled, &sm.SortOrder, &sm.PromptUpdatedAt, &sm.PromptUpdatedBy, &sm.ProjectID, &sm.IsCore, &projectsJSON); err != nil {
			return nil, fmt.Errorf("scan skill mapping: %w", err)
		}
		if err := json.Unmarshal(projectsJSON, &sm.Projects); err != nil {
			return nil, fmt.Errorf("decode skill projects: %w", err)
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}
