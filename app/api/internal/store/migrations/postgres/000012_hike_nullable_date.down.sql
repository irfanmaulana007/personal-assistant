-- Restore the NOT NULL constraint on hiked_on. Any dateless hikes recorded
-- while the column was nullable are backfilled from their created_at timestamp
-- first, so re-adding the constraint can't fail.

UPDATE hikes SET hiked_on = created_at WHERE hiked_on IS NULL;
ALTER TABLE hikes ALTER COLUMN hiked_on SET NOT NULL;
