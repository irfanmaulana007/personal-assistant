package store

import (
	"context"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
)

// ListNotionTargets returns the Notion database mappings for the active project,
// ordered by kind.
func (s *PostgresStore) ListNotionTargets(ctx context.Context) ([]NotionTarget, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, kind, database_id, name, url, created_at
		 FROM mcp_notion_targets WHERE project_id = $1 ORDER BY kind ASC`,
		authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list notion targets: %w", err)
	}
	defer rows.Close()
	var out []NotionTarget
	for rows.Next() {
		var t NotionTarget
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Kind, &t.DatabaseID, &t.Name, &t.URL, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notion target: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetNotionTarget maps a Notion database to the active project under a label. It
// is an upsert on (project_id, kind): re-mapping a kind refreshes the database
// id and cached name/url.
func (s *PostgresStore) SetNotionTarget(ctx context.Context, kind, databaseID, name, url string) (*NotionTarget, error) {
	pid := authctx.ProjectID(ctx)
	now := time.Now().UTC()
	var t NotionTarget
	err := s.pool.QueryRow(ctx,
		`INSERT INTO mcp_notion_targets (project_id, kind, database_id, name, url, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (project_id, kind)
		 DO UPDATE SET database_id = excluded.database_id, name = excluded.name, url = excluded.url
		 RETURNING id, project_id, kind, database_id, name, url, created_at`,
		pid, kind, databaseID, name, url, now,
	).Scan(&t.ID, &t.ProjectID, &t.Kind, &t.DatabaseID, &t.Name, &t.URL, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("set notion target: %w", err)
	}
	return &t, nil
}

// DeleteNotionTarget removes a Notion database mapping (by label) from the active
// project.
func (s *PostgresStore) DeleteNotionTarget(ctx context.Context, kind string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM mcp_notion_targets WHERE project_id = $1 AND kind = $2`,
		authctx.ProjectID(ctx), kind)
	if err != nil {
		return fmt.Errorf("delete notion target: %w", err)
	}
	return nil
}
