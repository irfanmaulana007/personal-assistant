-- Life Goals became the Bucket List: the table is renamed and each item now
-- carries a `category` and an optional `resolution_year` (set when the item is
-- flagged as that year's resolution). Existing rows are preserved and default
-- to the 'other' category with no resolution. Mirrors the SQLite migration.
ALTER TABLE life_goals RENAME TO bucket_list_items;
ALTER INDEX idx_life_goals_user RENAME TO idx_bucket_list_items_user;

ALTER TABLE bucket_list_items ADD COLUMN category TEXT NOT NULL DEFAULT 'other';
ALTER TABLE bucket_list_items ADD COLUMN resolution_year INTEGER;

CREATE INDEX idx_bucket_list_items_resolution ON bucket_list_items(user_id, resolution_year);
