-- Trello linking structure: a project can link many Trello workspaces
-- (organizations), and each linked workspace can link many boards. This replaces
-- the old single-board-per-project model (which lived as the plaintext
-- trello.workspace_id / trello.board_id rows in `settings`). Those settings are
-- kept as the "active board the assistant acts on" pointer; the tables below are
-- the browse/attach structure the integration page manages. Both tables are
-- project-scoped (project_id, mirroring every other domain table).
CREATE TABLE trello_workspaces (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   BIGINT NOT NULL DEFAULT 0,
    trello_id    TEXT   NOT NULL,              -- Trello organization (workspace) id
    name         TEXT   NOT NULL DEFAULT '',   -- display name, cached at attach time
    url          TEXT   NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, trello_id)
);
CREATE INDEX idx_trello_workspaces_project ON trello_workspaces(project_id);

CREATE TABLE trello_boards (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id    BIGINT NOT NULL DEFAULT 0,
    workspace_id  BIGINT NOT NULL REFERENCES trello_workspaces(id) ON DELETE CASCADE,
    trello_id     TEXT   NOT NULL,             -- Trello board id
    name          TEXT   NOT NULL DEFAULT '',
    url           TEXT   NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, trello_id)
);
CREATE INDEX idx_trello_boards_workspace ON trello_boards(workspace_id);
CREATE INDEX idx_trello_boards_project ON trello_boards(project_id);

-- Backfill existing single-board mappings so nothing is stranded. The old ids
-- live in `settings` as plaintext BYTEA under either the bare key (project 0) or
-- the project-scoped key `project:<pid>:trello.<field>`. Names are left blank and
-- get enriched live from the Trello API on next load.
INSERT INTO trello_workspaces (project_id, trello_id)
SELECT project_id, trello_id FROM (
    SELECT CASE WHEN key LIKE 'project:%' THEN split_part(key, ':', 2)::bigint ELSE 0 END AS project_id,
           NULLIF(convert_from(value, 'UTF8'), '') AS trello_id
    FROM settings
    WHERE key = 'trello.workspace_id' OR key LIKE 'project:%:trello.workspace_id'
) ws
WHERE trello_id IS NOT NULL
ON CONFLICT (project_id, trello_id) DO NOTHING;

-- A backfilled board links to its project's (single) migrated workspace. Boards
-- whose project had no workspace id are skipped here; their trello.board_id
-- setting still drives the assistant, and they can be re-attached in the UI.
INSERT INTO trello_boards (project_id, workspace_id, trello_id)
SELECT b.project_id, w.id, b.trello_id
FROM (
    SELECT CASE WHEN key LIKE 'project:%' THEN split_part(key, ':', 2)::bigint ELSE 0 END AS project_id,
           NULLIF(convert_from(value, 'UTF8'), '') AS trello_id
    FROM settings
    WHERE key = 'trello.board_id' OR key LIKE 'project:%:trello.board_id'
) b
JOIN trello_workspaces w ON w.project_id = b.project_id
WHERE b.trello_id IS NOT NULL
ON CONFLICT (project_id, trello_id) DO NOTHING;
