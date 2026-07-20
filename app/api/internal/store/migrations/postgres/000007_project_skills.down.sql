-- Reverts project-owned skills: drop every project fork (and its per-project
-- enable rows), then restore the flat global-only catalog with UNIQUE(key).

DELETE FROM project_skills
 WHERE skill_id IN (SELECT id FROM skills WHERE project_id IS NOT NULL);
DELETE FROM skills WHERE project_id IS NOT NULL;

DROP INDEX IF EXISTS skills_global_key_uniq;
DROP INDEX IF EXISTS skills_project_key_uniq;
DROP INDEX IF EXISTS idx_skills_project;

ALTER TABLE skills DROP COLUMN project_id;
ALTER TABLE skills ADD CONSTRAINT skills_key_key UNIQUE (key);
