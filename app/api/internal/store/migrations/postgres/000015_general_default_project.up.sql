-- Durable "default project": the single project the WhatsApp agent (and the
-- daily routine) act in for any chat with no more specific mapping. This
-- replaces the fragile "owner's first project" / unscoped project-0 fallback so
-- WhatsApp traffic always lands in one deliberately-chosen project — the
-- "General" project — unless an explicit whatsapp_mapping (personal number or
-- group binding) points it elsewhere.

ALTER TABLE projects ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;

-- At most one project may be the default at a time.
CREATE UNIQUE INDEX uniq_projects_single_default ON projects(is_default) WHERE is_default;

-- Adopt an existing "General" project as the default if one already exists.
UPDATE projects SET is_default = true
 WHERE id = (SELECT id FROM projects WHERE name = 'General' ORDER BY id ASC LIMIT 1)
   AND NOT EXISTS (SELECT 1 FROM projects WHERE is_default);

-- Otherwise seed a fresh "General" project as the default on deployments that
-- already have at least one user. It is owned by the platform owner (the first
-- superadmin, matching store.FirstAdmin — the account the WhatsApp agent runs
-- as; falls back to the lowest-id user), with no membership row: superadmins
-- manage every project via their global role (see 000008). Fresh installs have
-- no users at migration time; there the General project is created lazily on
-- first use by the application (store.EnsureDefaultProject).
INSERT INTO projects (name, slug, owner_user_id, is_default)
SELECT 'General', 'general', u.id, true
  FROM (SELECT id FROM users ORDER BY (role = 'superadmin') DESC, id ASC LIMIT 1) u
 WHERE NOT EXISTS (SELECT 1 FROM projects WHERE is_default)
   AND NOT EXISTS (SELECT 1 FROM projects WHERE slug = 'general');
