-- life_goals gained a `description` field (an enriched explanation of the goal),
-- alongside the existing short `note`. Mirrors the SQLite additive column.
ALTER TABLE life_goals ADD COLUMN description TEXT NOT NULL DEFAULT '';
