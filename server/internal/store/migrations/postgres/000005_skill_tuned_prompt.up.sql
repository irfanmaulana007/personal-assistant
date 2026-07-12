-- Auto-tuned prompt override for a skill. The shipped default lives in `prompt`
-- (re-seeded from code on every boot); `tuned_prompt`, when non-empty, is the
-- end-of-day self-tuner's improved version and is never overwritten by the seed.
-- Clearing it (back to '') reverts the skill to its shipped default.
ALTER TABLE skills ADD COLUMN tuned_prompt TEXT NOT NULL DEFAULT '';
