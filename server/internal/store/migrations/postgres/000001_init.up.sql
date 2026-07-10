-- PostgreSQL schema for the DataStore half of the hybrid backend (main data).
-- Log tables (activities, message_log, traces, trace_scores, tool_usage) live in
-- MongoDB and are intentionally absent here.
--
-- This is the effective schema: the SQLite base tables merged with the additive
-- columns that addColumnIfMissing applied over time, expressed with proper
-- Postgres types (BIGINT identity PKs, TIMESTAMPTZ, BOOLEAN, DOUBLE PRECISION,
-- and tsvector/GIN full-text instead of FTS5 virtual tables + triggers).

CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'member',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE contacts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    name       TEXT NOT NULL,
    phone      TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_contacts_user ON contacts(user_id);

CREATE TABLE life_goals (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    title      TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    done       BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    done_at    TIMESTAMPTZ
);
CREATE INDEX idx_life_goals_user ON life_goals(user_id, done);

CREATE TABLE trips (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    name        TEXT NOT NULL,
    destination TEXT NOT NULL DEFAULT '',
    budget      DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency    TEXT NOT NULL DEFAULT '',
    active      BOOLEAN NOT NULL DEFAULT true,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trips_user ON trips(user_id, active);

CREATE TABLE trip_expenses (
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id  BIGINT NOT NULL,
    trip_id  BIGINT NOT NULL,
    amount   DOUBLE PRECISION NOT NULL,
    currency TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT 'other',
    note     TEXT NOT NULL DEFAULT '',
    spent_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trip_expenses ON trip_expenses(user_id, trip_id);

CREATE TABLE hike_mountains (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_hike_mountains_user ON hike_mountains(user_id);

CREATE TABLE hike_tracks (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    mountain_id BIGINT NOT NULL,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_hike_tracks_user ON hike_tracks(user_id, mountain_id);

CREATE TABLE hike_participants (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_hike_participants_user ON hike_participants(user_id);

CREATE TABLE hikes (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    mountain_id   BIGINT NOT NULL,
    camped        BOOLEAN NOT NULL DEFAULT false,
    up_track_id   BIGINT NOT NULL DEFAULT 0,
    down_track_id BIGINT NOT NULL DEFAULT 0,
    days          INTEGER NOT NULL DEFAULT 0,
    nights        INTEGER NOT NULL DEFAULT 0,
    hiked_on      TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_hikes_user ON hikes(user_id, hiked_on);

CREATE TABLE hike_hikers (
    hike_id        BIGINT NOT NULL,
    participant_id BIGINT NOT NULL,
    PRIMARY KEY (hike_id, participant_id)
);

CREATE TABLE skills (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key             TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    prompt          TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL DEFAULT '',
    default_enabled BOOLEAN NOT NULL DEFAULT false,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_skills (
    user_id  BIGINT NOT NULL,
    skill_id BIGINT NOT NULL,
    enabled  BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, skill_id)
);

CREATE TABLE reminders (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id            BIGINT NOT NULL DEFAULT 0,
    title              TEXT NOT NULL DEFAULT '',
    message            TEXT NOT NULL,
    remind_at          TIMESTAMPTZ NOT NULL,
    repeat_mode        TEXT NOT NULL DEFAULT 'once',
    times              TEXT NOT NULL DEFAULT '',
    weekdays           TEXT NOT NULL DEFAULT '',
    day_of_month       INTEGER NOT NULL DEFAULT 0,
    once_date          TEXT NOT NULL DEFAULT '',
    event_at           TEXT NOT NULL DEFAULT '',
    offsets            TEXT NOT NULL DEFAULT '',
    enabled            BOOLEAN NOT NULL DEFAULT true,
    last_fired_at      TIMESTAMPTZ,
    calendar_conn      TEXT NOT NULL DEFAULT '',
    calendar_event_ids TEXT NOT NULL DEFAULT '',
    calendar_hash      TEXT NOT NULL DEFAULT '',
    notified           BOOLEAN NOT NULL DEFAULT false,
    cancelled          BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reminders_active ON reminders(remind_at) WHERE notified = false AND cancelled = false;
CREATE INDEX idx_reminders_owner_enabled ON reminders(user_id) WHERE enabled = true AND cancelled = false;

CREATE TABLE user_personas (
    user_id     BIGINT PRIMARY KEY,
    tone        TEXT NOT NULL DEFAULT 'balanced',
    emoji       TEXT NOT NULL DEFAULT 'occasional',
    length      TEXT NOT NULL DEFAULT 'balanced',
    personality TEXT NOT NULL DEFAULT 'balanced',
    name        TEXT NOT NULL DEFAULT '',
    custom      TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memories (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    content    TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    search     tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED
);
CREATE INDEX idx_memories_user ON memories(user_id);
CREATE INDEX idx_memories_search ON memories USING GIN (search);

CREATE TABLE notes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL DEFAULT 0,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    tags       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    search     tsvector GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(title, '') || ' ' || coalesce(content, '') || ' ' || coalesce(tags, ''))
    ) STORED
);
CREATE INDEX idx_notes_user ON notes(user_id);
CREATE INDEX idx_notes_search ON notes USING GIN (search);

CREATE TABLE oauth_tokens (
    service    TEXT PRIMARY KEY,
    token_data BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_prices (
    model         TEXT PRIMARY KEY,
    input_per_1m  DOUBLE PRECISION NOT NULL DEFAULT 0,
    output_per_1m DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
