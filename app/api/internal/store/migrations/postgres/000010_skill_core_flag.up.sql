-- Add an is_core flag to skills. Core skills are auto-available to every
-- project (like any global skill) and are pinned into the "Core" classification
-- regardless of how many projects use them, but remain enable/disable-able per
-- project. The flag is superadmin-managed after this migration; the boot seed
-- sets it only on first insert and never overwrites an admin's later choice.
ALTER TABLE skills ADD COLUMN IF NOT EXISTS is_core BOOLEAN NOT NULL DEFAULT false;

-- Seed the initial core set: the system automation skills that every project
-- should always have available.
UPDATE skills SET is_core = true
 WHERE project_id IS NULL AND key IN ('self_tuning', 'auto_triage');
