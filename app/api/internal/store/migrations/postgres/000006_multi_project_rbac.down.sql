-- Reverse of 000006: drop the per-project scoping columns, the RBAC/project
-- tables, and revert the global role upgrade.

DROP INDEX IF EXISTS idx_notes_project;
ALTER TABLE notes DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_memories_project;
ALTER TABLE memories DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_reminders_project;
ALTER TABLE reminders DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_hikes_project;
ALTER TABLE hikes DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_hike_participants_project;
ALTER TABLE hike_participants DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_hike_tracks_project;
ALTER TABLE hike_tracks DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_hike_mountains_project;
ALTER TABLE hike_mountains DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_trip_expenses_project;
ALTER TABLE trip_expenses DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_trips_project;
ALTER TABLE trips DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_bucket_list_items_project;
ALTER TABLE bucket_list_items DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS idx_contacts_project;
ALTER TABLE contacts DROP COLUMN IF EXISTS project_id;

UPDATE users SET role = 'admin' WHERE role = 'superadmin';

DROP TABLE IF EXISTS whatsapp_mappings;
DROP TABLE IF EXISTS project_features;
DROP TABLE IF EXISTS feature_skills;
DROP TABLE IF EXISTS features;
DROP TABLE IF EXISTS project_skills;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
