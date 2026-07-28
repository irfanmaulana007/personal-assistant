-- Drop the default-project marker. The General project row (if one was created)
-- is intentionally left in place so no project data is stranded on a rollback.
DROP INDEX IF EXISTS uniq_projects_single_default;
ALTER TABLE projects DROP COLUMN IF EXISTS is_default;
