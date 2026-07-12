DROP INDEX idx_bucket_list_items_resolution;
ALTER TABLE bucket_list_items DROP COLUMN resolution_year;
ALTER TABLE bucket_list_items DROP COLUMN category;

ALTER INDEX idx_bucket_list_items_user RENAME TO idx_life_goals_user;
ALTER TABLE bucket_list_items RENAME TO life_goals;
