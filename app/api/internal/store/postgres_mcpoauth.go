package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/jackc/pgx/v5"
)

// CreateMCPOAuthPending records an in-flight OAuth authorization. The project id
// is taken from the row (set by the caller at initiate time), not from context.
func (s *PostgresStore) CreateMCPOAuthPending(ctx context.Context, p MCPOAuthPending) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO mcp_oauth_pending
		   (state, project_id, provider, verifier_enc, client_id, client_secret_enc,
		    auth_endpoint, token_endpoint, redirect_uri, resource, scopes, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		p.State, p.ProjectID, p.Provider, p.VerifierEnc, p.ClientID, p.ClientSecretEnc,
		p.AuthEndpoint, p.TokenEndpoint, p.RedirectURI, p.Resource, p.Scopes, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create mcp oauth pending: %w", err)
	}
	return nil
}

// GetMCPOAuthPending looks up a pending authorization by state. Returns nil when
// not found. Not project-scoped — the callback has no app session.
func (s *PostgresStore) GetMCPOAuthPending(ctx context.Context, state string) (*MCPOAuthPending, error) {
	var p MCPOAuthPending
	err := s.pool.QueryRow(ctx,
		`SELECT state, project_id, provider, verifier_enc, client_id, client_secret_enc,
		        auth_endpoint, token_endpoint, redirect_uri, resource, scopes, created_at
		 FROM mcp_oauth_pending WHERE state = $1`, state,
	).Scan(&p.State, &p.ProjectID, &p.Provider, &p.VerifierEnc, &p.ClientID, &p.ClientSecretEnc,
		&p.AuthEndpoint, &p.TokenEndpoint, &p.RedirectURI, &p.Resource, &p.Scopes, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get mcp oauth pending: %w", err)
	}
	return &p, nil
}

// DeleteMCPOAuthPending removes a pending authorization by state.
func (s *PostgresStore) DeleteMCPOAuthPending(ctx context.Context, state string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM mcp_oauth_pending WHERE state = $1`, state); err != nil {
		return fmt.Errorf("delete mcp oauth pending: %w", err)
	}
	return nil
}

// UpsertMCPOAuthConnection stores (or replaces) a completed OAuth connection. The
// project id comes from the row (resolved from the pending row at callback time).
func (s *PostgresStore) UpsertMCPOAuthConnection(ctx context.Context, c MCPOAuthConnection) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO mcp_oauth_connections
		   (project_id, provider, client_id, client_secret_enc, token_endpoint,
		    access_token_enc, refresh_token_enc, expiry, scopes, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		 ON CONFLICT (project_id, provider) DO UPDATE SET
		    client_id = excluded.client_id,
		    client_secret_enc = excluded.client_secret_enc,
		    token_endpoint = excluded.token_endpoint,
		    access_token_enc = excluded.access_token_enc,
		    refresh_token_enc = excluded.refresh_token_enc,
		    expiry = excluded.expiry,
		    scopes = excluded.scopes,
		    updated_at = excluded.updated_at`,
		c.ProjectID, c.Provider, c.ClientID, c.ClientSecretEnc, c.TokenEndpoint,
		c.AccessTokenEnc, c.RefreshTokenEnc, c.Expiry, c.Scopes, now)
	if err != nil {
		return fmt.Errorf("upsert mcp oauth connection: %w", err)
	}
	return nil
}

// GetMCPOAuthConnection returns the active project's connection for a provider,
// or nil when none. Project-scoped via context.
func (s *PostgresStore) GetMCPOAuthConnection(ctx context.Context, provider string) (*MCPOAuthConnection, error) {
	var c MCPOAuthConnection
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, provider, client_id, client_secret_enc, token_endpoint,
		        access_token_enc, refresh_token_enc, expiry, scopes, created_at, updated_at
		 FROM mcp_oauth_connections WHERE project_id = $1 AND provider = $2`,
		authctx.ProjectID(ctx), provider,
	).Scan(&c.ID, &c.ProjectID, &c.Provider, &c.ClientID, &c.ClientSecretEnc, &c.TokenEndpoint,
		&c.AccessTokenEnc, &c.RefreshTokenEnc, &c.Expiry, &c.Scopes, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get mcp oauth connection: %w", err)
	}
	return &c, nil
}

// UpdateMCPOAuthToken persists refreshed tokens for a connection. Takes explicit
// ids because a refresh may run outside a project-scoped request path.
func (s *PostgresStore) UpdateMCPOAuthToken(ctx context.Context, projectID int64, provider string, accessEnc, refreshEnc []byte, expiry *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mcp_oauth_connections
		 SET access_token_enc = $3, refresh_token_enc = $4, expiry = $5, updated_at = now()
		 WHERE project_id = $1 AND provider = $2`,
		projectID, provider, accessEnc, refreshEnc, expiry)
	if err != nil {
		return fmt.Errorf("update mcp oauth token: %w", err)
	}
	return nil
}

// DeleteMCPOAuthConnection removes the active project's connection for a provider.
func (s *PostgresStore) DeleteMCPOAuthConnection(ctx context.Context, provider string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM mcp_oauth_connections WHERE project_id = $1 AND provider = $2`,
		authctx.ProjectID(ctx), provider)
	if err != nil {
		return fmt.Errorf("delete mcp oauth connection: %w", err)
	}
	return nil
}
