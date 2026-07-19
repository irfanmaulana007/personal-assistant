package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// seedFeatures upserts the code-owned feature catalog and rebuilds the
// feature→skill attachments from featureSkillSeed. Idempotent; NewPostgres wires
// the call after seedSkills so the skill ids exist.
func (s *PostgresStore) seedFeatures(ctx context.Context) error {
	for _, f := range featureSeed {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO features (key, name, description, sort_order, default_enabled)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (key) DO UPDATE SET
			   name = EXCLUDED.name,
			   description = EXCLUDED.description,
			   sort_order = EXCLUDED.sort_order,
			   default_enabled = EXCLUDED.default_enabled`,
			f.Key, f.Name, f.Description, f.SortOrder, f.DefaultEnabled,
		); err != nil {
			return fmt.Errorf("seed feature %s: %w", f.Key, err)
		}
	}
	// feature_skills is a code-owned mapping; rebuild it wholesale each boot.
	if _, err := s.pool.Exec(ctx, `DELETE FROM feature_skills`); err != nil {
		return fmt.Errorf("clear feature_skills: %w", err)
	}
	for fkey, skeys := range featureSkillSeed {
		for _, sk := range skeys {
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO feature_skills (feature_id, skill_id)
				 SELECT f.id, s.id FROM features f, skills s
				 WHERE f.key = $1 AND s.key = $2 AND s.project_id IS NULL
				 ON CONFLICT DO NOTHING`,
				fkey, sk,
			); err != nil {
				return fmt.Errorf("seed feature_skill %s/%s: %w", fkey, sk, err)
			}
		}
	}
	return nil
}

// --- Projects ---

var slugStripRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a project name into a URL-safe slug: lowercase, with runs of
// non-alphanumeric characters collapsed to single dashes and trimmed. Falls back
// to "project" when the name has no alphanumeric characters.
func slugify(name string) string {
	s := slugStripRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "project"
	}
	return s
}

// reservedSlugs are top-level route words the client owns (the global shell). A
// project may not take one, or its /:slug shell would be shadowed by that route.
var reservedSlugs = map[string]bool{
	"overview": true, "account": true, "projects": true, "settings": true,
	"login": true, "api": true,
}

// uniqueProjectSlug returns base if it is free and unreserved, otherwise base-2,
// base-3, … until it finds a slug no project holds and no route reserves.
func (s *PostgresStore) uniqueProjectSlug(ctx context.Context, base string) (string, error) {
	slug := base
	for i := 2; ; i++ {
		if !reservedSlugs[slug] {
			var exists bool
			if err := s.pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM projects WHERE slug = $1)`, slug,
			).Scan(&exists); err != nil {
				return "", fmt.Errorf("check project slug: %w", err)
			}
			if !exists {
				return slug, nil
			}
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func (s *PostgresStore) CreateProject(ctx context.Context, name string, ownerUserID int64) (*Project, error) {
	slug, err := s.uniqueProjectSlug(ctx, slugify(name))
	if err != nil {
		return nil, err
	}
	var p Project
	err = s.pool.QueryRow(ctx,
		`INSERT INTO projects (name, slug, owner_user_id) VALUES ($1, $2, $3)
		 RETURNING id, name, slug, owner_user_id, created_at`,
		name, slug, ownerUserID,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerUserID, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) GetProject(ctx context.Context, id int64) (*Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, owner_user_id, created_at FROM projects WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerUserID, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, owner_user_id, created_at FROM projects ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerUserID, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListProjectsForUser(ctx context.Context, userID int64) ([]ProjectSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT p.id, p.name, p.slug, p.owner_user_id, p.created_at, pm.role,
		        (SELECT count(*) FROM project_members m WHERE m.project_id = p.id) AS member_count
		 FROM projects p
		 JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $1
		 ORDER BY p.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects for user: %w", err)
	}
	defer rows.Close()

	var out []ProjectSummary
	for rows.Next() {
		var ps ProjectSummary
		if err := rows.Scan(&ps.ID, &ps.Name, &ps.Slug, &ps.OwnerUserID, &ps.CreatedAt, &ps.Role, &ps.MemberCount); err != nil {
			return nil, fmt.Errorf("scan project summary: %w", err)
		}
		out = append(out, ps)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateProjectName(ctx context.Context, id int64, name string) error {
	_, err := s.pool.Exec(ctx, `UPDATE projects SET name = $2 WHERE id = $1`, id, name)
	return err
}

// projectScopedTables are the domain tables whose rows carry a project_id and
// must be removed when a project is hard-deleted.
var projectScopedTables = []string{
	"contacts", "bucket_list_items", "trips", "trip_expenses",
	"hike_mountains", "hike_tracks", "hike_participants", "hikes",
	"reminders", "memories", "notes",
}

func (s *PostgresStore) DeleteProject(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete project: %w", err)
	}
	defer tx.Rollback(ctx)

	// hike_hikers has no project_id; clear the rows belonging to this project's hikes first.
	if _, err := tx.Exec(ctx,
		`DELETE FROM hike_hikers WHERE hike_id IN (SELECT id FROM hikes WHERE project_id = $1)`, id); err != nil {
		return fmt.Errorf("delete hike_hikers: %w", err)
	}
	for _, t := range projectScopedTables {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE project_id = $1`, t), id); err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
	}
	// `skills` here only ever matches this project's forks — global rows carry a
	// NULL project_id, so the shared catalog is untouched. project_skills is
	// cleared first so no toggle row dangles past its skill.
	for _, t := range []string{"project_skills", "skills", "project_features", "project_members", "whatsapp_mappings"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE project_id = $1`, t), id); err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return tx.Commit(ctx)
}

// --- Project membership ---

func (s *PostgresStore) ListProjectMembers(ctx context.Context, projectID int64) ([]ProjectMemberDetail, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT pm.user_id, u.email, u.name, pm.role, pm.created_at
		 FROM project_members pm
		 JOIN users u ON u.id = pm.user_id
		 WHERE pm.project_id = $1
		 ORDER BY pm.role ASC, u.email ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	defer rows.Close()

	var out []ProjectMemberDetail
	for rows.Next() {
		var m ProjectMemberDetail
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetProjectRole(ctx context.Context, projectID, userID int64) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get project role: %w", err)
	}
	return role, nil
}

func (s *PostgresStore) AddProjectMember(ctx context.Context, projectID, userID int64, role string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		projectID, userID, role,
	)
	return err
}

func (s *PostgresStore) UpdateProjectMemberRole(ctx context.Context, projectID, userID int64, role string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE project_members SET role = $3 WHERE project_id = $1 AND user_id = $2`,
		projectID, userID, role,
	)
	return err
}

func (s *PostgresStore) RemoveProjectMember(ctx context.Context, projectID, userID int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, userID)
	return err
}

func (s *PostgresStore) CountProjectAdmins(ctx context.Context, projectID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM project_members WHERE project_id = $1 AND role = 'admin'`, projectID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count project admins: %w", err)
	}
	return n, nil
}

// --- Per-project skills ---

// projectSkillEffective is the SQL boolean expression for a skill's effective
// enabled state in a project: the per-project override (else the skill default)
// AND the feature gate (a skill under a disabled feature is off).
const projectSkillEffective = `(COALESCE(ps.enabled, s.default_enabled) AND COALESCE(pf.enabled, f.default_enabled, true))`

// projectSkillJoins resolves a project's effective skill set. `gt` is the global
// "twin": for a global skill it is the row itself; for a project fork it is the
// global skill sharing its key (NULL for a fork with no global twin). The
// feature gate is keyed off the twin so a fork inherits its global skill's
// feature. project_skills carries the per-project enable flag for both scopes.
const projectSkillJoins = `
	 FROM skills s
	 LEFT JOIN skills gt ON gt.project_id IS NULL AND gt.key = s.key
	 LEFT JOIN project_skills ps ON ps.skill_id = s.id AND ps.project_id = $1
	 LEFT JOIN feature_skills fs ON fs.skill_id = gt.id
	 LEFT JOIN features f ON f.id = fs.feature_id
	 LEFT JOIN project_features pf ON pf.feature_id = f.id AND pf.project_id = $1`

// projectSkillScope restricts a listing to the project's own forks plus every
// global skill not shadowed by a fork of the same key in that project.
const projectSkillScope = `
	 WHERE (s.project_id IS NULL OR s.project_id = $1)
	   AND NOT (s.project_id IS NULL AND EXISTS (
	         SELECT 1 FROM skills o WHERE o.project_id = $1 AND o.key = s.key))`

func (s *PostgresStore) ListProjectSkills(ctx context.Context, projectID int64) ([]UserSkill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.key, s.name, s.description, s.prompt, s.tuned_prompt, s.category, s.default_enabled, s.sort_order,
		        s.prompt_updated_at, s.prompt_updated_by, s.project_id, s.is_core, `+projectSkillEffective+` AS effective`+
			projectSkillJoins+projectSkillScope+`
		 ORDER BY s.sort_order ASC, s.id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project skills: %w", err)
	}
	defer rows.Close()

	var out []UserSkill
	for rows.Next() {
		var us UserSkill
		if err := rows.Scan(&us.ID, &us.Key, &us.Name, &us.Description, &us.Prompt, &us.TunedPrompt, &us.Category, &us.DefaultEnabled, &us.SortOrder, &us.PromptUpdatedAt, &us.PromptUpdatedBy, &us.ProjectID, &us.IsCore, &us.Enabled); err != nil {
			return nil, fmt.Errorf("scan project skill: %w", err)
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SetProjectSkillEnabled(ctx context.Context, projectID, skillID int64, enabled bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO project_skills (project_id, skill_id, enabled) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, skill_id) DO UPDATE SET enabled = EXCLUDED.enabled`,
		projectID, skillID, enabled,
	)
	return err
}

func (s *PostgresStore) EnabledProjectSkillKeys(ctx context.Context, projectID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.key`+projectSkillJoins+projectSkillScope+`
		   AND `+projectSkillEffective+` = true`, projectID)
	if err != nil {
		return nil, fmt.Errorf("enabled project skill keys: %w", err)
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

// --- Features ---

func (s *PostgresStore) ListFeatures(ctx context.Context) ([]Feature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, key, name, description, sort_order, default_enabled FROM features ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()

	var out []Feature
	for rows.Next() {
		var f Feature
		if err := rows.Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.SortOrder, &f.DefaultEnabled); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetFeature(ctx context.Context, id int64) (*Feature, error) {
	var f Feature
	err := s.pool.QueryRow(ctx,
		`SELECT id, key, name, description, sort_order, default_enabled FROM features WHERE id = $1`, id,
	).Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.SortOrder, &f.DefaultEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get feature: %w", err)
	}
	return &f, nil
}

func (s *PostgresStore) ListProjectFeatures(ctx context.Context, projectID int64) ([]ProjectFeature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT f.id, f.key, f.name, f.description, f.sort_order, f.default_enabled,
		        COALESCE(pf.enabled, f.default_enabled) AS effective,
		        COALESCE(array_agg(s.key ORDER BY s.sort_order) FILTER (WHERE s.key IS NOT NULL), '{}') AS skill_keys
		 FROM features f
		 LEFT JOIN project_features pf ON pf.feature_id = f.id AND pf.project_id = $1
		 LEFT JOIN feature_skills fs ON fs.feature_id = f.id
		 LEFT JOIN skills s ON s.id = fs.skill_id
		 GROUP BY f.id, pf.enabled
		 ORDER BY f.sort_order ASC, f.id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project features: %w", err)
	}
	defer rows.Close()

	var out []ProjectFeature
	for rows.Next() {
		var pf ProjectFeature
		if err := rows.Scan(&pf.ID, &pf.Key, &pf.Name, &pf.Description, &pf.SortOrder, &pf.DefaultEnabled, &pf.Enabled, &pf.SkillKeys); err != nil {
			return nil, fmt.Errorf("scan project feature: %w", err)
		}
		out = append(out, pf)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SetProjectFeatureEnabled(ctx context.Context, projectID, featureID int64, enabled bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO project_features (project_id, feature_id, enabled) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, feature_id) DO UPDATE SET enabled = EXCLUDED.enabled`,
		projectID, featureID, enabled,
	)
	return err
}

// --- WhatsApp mappings ---

func (s *PostgresStore) ListWhatsAppMappings(ctx context.Context) ([]WhatsAppMapping, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, jid, kind, project_id, role, user_id, label, created_at
		 FROM whatsapp_mappings ORDER BY kind ASC, label ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list whatsapp mappings: %w", err)
	}
	defer rows.Close()

	var out []WhatsAppMapping
	for rows.Next() {
		var m WhatsAppMapping
		if err := rows.Scan(&m.ID, &m.JID, &m.Kind, &m.ProjectID, &m.Role, &m.UserID, &m.Label, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan whatsapp mapping: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetWhatsAppMapping(ctx context.Context, jid string) (*WhatsAppMapping, error) {
	var m WhatsAppMapping
	err := s.pool.QueryRow(ctx,
		`SELECT id, jid, kind, project_id, role, user_id, label, created_at
		 FROM whatsapp_mappings WHERE jid = $1`, jid,
	).Scan(&m.ID, &m.JID, &m.Kind, &m.ProjectID, &m.Role, &m.UserID, &m.Label, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get whatsapp mapping: %w", err)
	}
	return &m, nil
}

func (s *PostgresStore) CreateWhatsAppMapping(ctx context.Context, m WhatsAppMapping) (*WhatsAppMapping, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO whatsapp_mappings (jid, kind, project_id, role, user_id, label)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (jid) DO UPDATE SET
		   kind = EXCLUDED.kind, project_id = EXCLUDED.project_id, role = EXCLUDED.role,
		   user_id = EXCLUDED.user_id, label = EXCLUDED.label
		 RETURNING id, jid, kind, project_id, role, user_id, label, created_at`,
		m.JID, m.Kind, m.ProjectID, m.Role, m.UserID, m.Label,
	).Scan(&m.ID, &m.JID, &m.Kind, &m.ProjectID, &m.Role, &m.UserID, &m.Label, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create whatsapp mapping: %w", err)
	}
	return &m, nil
}

func (s *PostgresStore) UpdateWhatsAppMapping(ctx context.Context, id int64, m WhatsAppMapping) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE whatsapp_mappings SET jid = $2, kind = $3, project_id = $4, role = $5, user_id = $6, label = $7
		 WHERE id = $1`,
		id, m.JID, m.Kind, m.ProjectID, m.Role, m.UserID, m.Label,
	)
	return err
}

func (s *PostgresStore) DeleteWhatsAppMapping(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM whatsapp_mappings WHERE id = $1`, id)
	return err
}
