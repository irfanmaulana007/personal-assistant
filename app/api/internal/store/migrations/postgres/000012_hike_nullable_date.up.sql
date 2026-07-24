-- Allow a logged hike to have no date.
--
-- Until now hiked_on was NOT NULL and any flow that didn't get an explicit date
-- fabricated one (the chat logger and the web form both defaulted to "now").
-- That silently invented a hiking date the user never gave. Making the column
-- nullable lets a hike be recorded without a date and filled in later (via the
-- hike_update tool or the web form), instead of storing a wrong one.

ALTER TABLE hikes ALTER COLUMN hiked_on DROP NOT NULL;
