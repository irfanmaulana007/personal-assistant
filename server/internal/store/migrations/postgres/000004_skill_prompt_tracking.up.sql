-- Per-skill prompts become admin-editable. Two columns track the last edit:
-- prompt_updated_at is NULL while the prompt is still the code-owned default
-- (managed by the boot seed); once an admin customizes it both columns are set
-- and the seed's ON CONFLICT stops overwriting the prompt. Mirrors the SQLite
-- additive migration. Existing rows keep the default (NULL updated_at).
ALTER TABLE skills ADD COLUMN prompt_updated_at TIMESTAMPTZ;
ALTER TABLE skills ADD COLUMN prompt_updated_by TEXT NOT NULL DEFAULT '';
