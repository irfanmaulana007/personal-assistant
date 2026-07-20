-- Project-owned skills.
--
-- Splits the flat skills catalog into two scopes:
--   * global skills   (project_id IS NULL): the code-seeded catalog, shared by
--     every project, whose prompt is superadmin-owned.
--   * project skills  (project_id set):     a fork a project admin created from
--     a global skill and customized. A project fork SHADOWS the global skill of
--     the same key for that project, so the agent injects the project's prompt.
--
-- Enable/disable for both scopes continues to live in project_skills.

ALTER TABLE skills ADD COLUMN project_id BIGINT; -- NULL = global (code-seeded)

-- Replace the single UNIQUE(key) with scope-aware partial uniques: global keys
-- are unique among globals; a project's keys are unique within that project.
-- Dropping the old constraint is mandatory — it is on `key` alone and would
-- otherwise block a fork from reusing its global skill's key.
ALTER TABLE skills DROP CONSTRAINT IF EXISTS skills_key_key;
CREATE UNIQUE INDEX skills_global_key_uniq  ON skills(key) WHERE project_id IS NULL;
CREATE UNIQUE INDEX skills_project_key_uniq ON skills(project_id, key) WHERE project_id IS NOT NULL;
CREATE INDEX idx_skills_project ON skills(project_id);
