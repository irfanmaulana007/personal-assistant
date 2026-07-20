DROP INDEX IF EXISTS projects_slug_uniq;
ALTER TABLE projects DROP COLUMN IF EXISTS slug;
