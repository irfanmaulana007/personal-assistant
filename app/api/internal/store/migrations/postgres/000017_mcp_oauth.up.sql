-- OAuth 2.1 connections for MCP servers whose hosted endpoints require OAuth
-- (Notion, Railway — they reject static tokens). Cloudflare keeps using a token
-- stored in `settings`, so it is not represented here.
--
-- Two tables, both project-scoped (mirroring every other domain table). Secrets
-- (PKCE verifier, client secret, access/refresh tokens) are stored encrypted as
-- BYTEA by the mcpoauth service; the store treats them as opaque bytes.

-- An in-flight authorization, keyed by the opaque `state` we mint. Looked up by
-- state at the OAuth callback, which carries no app session. Short-lived.
CREATE TABLE mcp_oauth_pending (
    state              TEXT PRIMARY KEY,
    project_id         BIGINT NOT NULL DEFAULT 0,
    provider           TEXT   NOT NULL,
    verifier_enc       BYTEA  NOT NULL,               -- PKCE code_verifier
    client_id          TEXT   NOT NULL,
    client_secret_enc  BYTEA  NOT NULL DEFAULT '',    -- DCR secret (empty for public clients)
    auth_endpoint      TEXT   NOT NULL,
    token_endpoint     TEXT   NOT NULL,
    redirect_uri       TEXT   NOT NULL,
    resource           TEXT   NOT NULL DEFAULT '',    -- RFC 8707 resource indicator (the MCP endpoint)
    scopes             TEXT   NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mcp_oauth_pending_created ON mcp_oauth_pending(created_at);

-- A completed OAuth connection for a (project, provider).
CREATE TABLE mcp_oauth_connections (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id         BIGINT NOT NULL DEFAULT 0,
    provider           TEXT   NOT NULL,
    client_id          TEXT   NOT NULL,
    client_secret_enc  BYTEA  NOT NULL DEFAULT '',
    token_endpoint     TEXT   NOT NULL,
    access_token_enc   BYTEA  NOT NULL,
    refresh_token_enc  BYTEA  NOT NULL DEFAULT '',
    expiry             TIMESTAMPTZ,
    scopes             TEXT   NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, provider)
);
CREATE INDEX idx_mcp_oauth_connections_project ON mcp_oauth_connections(project_id);
