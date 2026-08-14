-- Notion MCP database mapping: a project maps labelled Notion databases
-- ("spaces") so the MCP-backed agent knows which database is the task tracker,
-- which is the issue tracker, and so on. Project-scoped (project_id, mirroring
-- every other domain table). `kind` is a free label, unique per project, so the
-- mapping can grow beyond task/issue.
CREATE TABLE mcp_notion_targets (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   BIGINT NOT NULL DEFAULT 0,
    kind         TEXT   NOT NULL,              -- 'task' | 'issue' | … (free label)
    database_id  TEXT   NOT NULL,              -- Notion database (data source) id
    name         TEXT   NOT NULL DEFAULT '',   -- display name, cached at map time
    url          TEXT   NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind)
);
CREATE INDEX idx_mcp_notion_targets_project ON mcp_notion_targets(project_id);
