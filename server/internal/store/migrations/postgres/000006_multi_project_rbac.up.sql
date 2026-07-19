-- Multi-Project Support with RBAC.
--
-- Introduces projects as the tenancy boundary. Every user-owned domain row is
-- re-scoped from a flat user_id to a project_id (user_id is kept as the creator
-- stamp). The global users.role is upgraded from 'admin'/'member' to
-- 'superadmin'/'member'; per-project roles ('admin'/'member') live in
-- project_members. Existing users are each migrated to one personal project on
-- which they are the project admin, and their existing per-user skill toggles
-- carry over to that project so nothing regresses.

-- 1. RBAC / project tables -------------------------------------------------

CREATE TABLE projects (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT NOT NULL,
    owner_user_id BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_projects_owner ON projects(owner_user_id);

-- who can access a project, and as what ('admin' | 'member')
CREATE TABLE project_members (
    project_id BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX idx_project_members_user ON project_members(user_id);

-- per-project skill override (mirrors user_skills)
CREATE TABLE project_skills (
    project_id BIGINT NOT NULL,
    skill_id   BIGINT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (project_id, skill_id)
);

-- catalog of nav/menu modules ("features"); code-seeded like skills
CREATE TABLE features (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key             TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    sort_order      INTEGER NOT NULL DEFAULT 0,
    default_enabled BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- which skills belong to a feature (0..n per feature)
CREATE TABLE feature_skills (
    feature_id BIGINT NOT NULL,
    skill_id   BIGINT NOT NULL,
    PRIMARY KEY (feature_id, skill_id)
);

-- per-project feature override; absent row ⇒ features.default_enabled
CREATE TABLE project_features (
    project_id BIGINT NOT NULL,
    feature_id BIGINT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (project_id, feature_id)
);

-- Maps a WhatsApp identity (a group JID or a personal phone/JID) to the project
-- (and, for personal chats, the role) the agent acts as when a message arrives
-- from it. Group mappings always scope to their project; a personal mapping may
-- grant 'superadmin' so the owner's 1:1 chat can reach every skill. Group
-- messages never confer superadmin regardless of the sender.
CREATE TABLE whatsapp_mappings (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    jid        TEXT NOT NULL UNIQUE,        -- group JID or personal phone/JID
    kind       TEXT NOT NULL,               -- 'group' | 'personal'
    project_id BIGINT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'member', -- role the agent acts as ('superadmin' allowed for personal only)
    user_id    BIGINT NOT NULL DEFAULT 0,   -- user identity attributed to this chat (personal), 0 if none
    label      TEXT NOT NULL DEFAULT '',    -- human-friendly name for the admin UI
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_whatsapp_mappings_project ON whatsapp_mappings(project_id);

-- 2. Global role upgrade: admin -> superadmin ------------------------------

UPDATE users SET role = 'superadmin' WHERE role = 'admin';

-- 3. One personal project per existing user, owner = project admin ---------

INSERT INTO projects (name, owner_user_id)
    SELECT COALESCE(NULLIF(name, ''), email) || ' — Personal', id
      FROM users;

INSERT INTO project_members (project_id, user_id, role)
    SELECT p.id, p.owner_user_id, 'admin'
      FROM projects p;

-- 4. Carry existing per-user skill config over to the personal project -----

INSERT INTO project_skills (project_id, skill_id, enabled)
    SELECT p.id, us.skill_id, us.enabled
      FROM user_skills us
      JOIN projects p ON p.owner_user_id = us.user_id;

-- 5. Re-scope domain tables to project_id ----------------------------------
-- project_id BIGINT NOT NULL DEFAULT 0; backfill each row to its owner's
-- personal project (the projects rows above already exist in this same
-- transaction), then index it.

ALTER TABLE contacts ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE contacts c SET project_id = p.id FROM projects p WHERE p.owner_user_id = c.user_id;
CREATE INDEX idx_contacts_project ON contacts(project_id);

ALTER TABLE bucket_list_items ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE bucket_list_items t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_bucket_list_items_project ON bucket_list_items(project_id);

ALTER TABLE trips ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE trips t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_trips_project ON trips(project_id);

ALTER TABLE trip_expenses ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE trip_expenses t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_trip_expenses_project ON trip_expenses(project_id);

ALTER TABLE hike_mountains ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE hike_mountains t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_hike_mountains_project ON hike_mountains(project_id);

ALTER TABLE hike_tracks ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE hike_tracks t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_hike_tracks_project ON hike_tracks(project_id);

ALTER TABLE hike_participants ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE hike_participants t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_hike_participants_project ON hike_participants(project_id);

ALTER TABLE hikes ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE hikes t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_hikes_project ON hikes(project_id);

ALTER TABLE reminders ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE reminders t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_reminders_project ON reminders(project_id);

ALTER TABLE memories ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE memories t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_memories_project ON memories(project_id);

ALTER TABLE notes ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
UPDATE notes t SET project_id = p.id FROM projects p WHERE p.owner_user_id = t.user_id;
CREATE INDEX idx_notes_project ON notes(project_id);
