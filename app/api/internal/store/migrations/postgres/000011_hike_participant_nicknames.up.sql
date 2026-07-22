-- Give each hiking participant a stored list of nicknames / aliases (panggilan)
-- alongside their canonical name (nama panjang).
--
-- Until now the only way a person's alternate spellings got folded together was
-- the runtime fuzzy matcher, which silently remapped distinct people onto each
-- other (e.g. logging "Ali" reused an existing "Abi"). Nicknames make aliasing
-- explicit and user-controlled: a logged name only resolves to a participant
-- when it exactly matches that participant's name or one of their recorded
-- nicknames. Merging two duplicate participants records the absorbed name here
-- so future logs resolve to the surviving record.

ALTER TABLE hike_participants
    ADD COLUMN nicknames TEXT[] NOT NULL DEFAULT '{}';
