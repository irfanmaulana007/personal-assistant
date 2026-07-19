-- Human-readable, URL-safe slug for each project.
--
-- The client scopes pages by a `/:slug/...` URL prefix, so every project needs a
-- stable, unique slug. Slugs are generated once at create time and are immutable
-- (renaming a project does not change its slug, so existing URLs never break).

ALTER TABLE projects ADD COLUMN slug TEXT;

-- Backfill a URL-safe slug from each project's name: lowercase, non-alphanumeric
-- runs collapsed to single dashes and trimmed. Empty results fall back to "project".
UPDATE projects
SET slug = COALESCE(
    NULLIF(trim(both '-' from lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g'))), ''),
    'project'
);

-- De-duplicate the backfill: keep the lowest-id occurrence clean, suffix the rest
-- with their id so the whole set is unique. New projects stay unique via the
-- application's create-time slug generation.
UPDATE projects p
SET slug = p.slug || '-' || p.id
FROM (
    SELECT id, row_number() OVER (PARTITION BY slug ORDER BY id) AS rn
    FROM projects
) d
WHERE p.id = d.id AND d.rn > 1;

ALTER TABLE projects ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX projects_slug_uniq ON projects(slug);
