package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
)

// ListTrelloWorkspaces returns the Trello workspaces linked to the active
// project, newest first.
func (s *PostgresStore) ListTrelloWorkspaces(ctx context.Context) ([]TrelloWorkspaceLink, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, trello_id, name, url, created_at
		 FROM trello_workspaces WHERE project_id = $1 ORDER BY created_at DESC, id DESC`,
		authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list trello workspaces: %w", err)
	}
	defer rows.Close()
	var out []TrelloWorkspaceLink
	for rows.Next() {
		var w TrelloWorkspaceLink
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.TrelloID, &w.Name, &w.URL, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan trello workspace: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AttachTrelloWorkspace links a Trello workspace to the active project. It is an
// upsert on (project_id, trello_id): re-attaching refreshes the cached name/url.
func (s *PostgresStore) AttachTrelloWorkspace(ctx context.Context, trelloID, name, url string) (*TrelloWorkspaceLink, error) {
	pid := authctx.ProjectID(ctx)
	now := time.Now().UTC()
	var w TrelloWorkspaceLink
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trello_workspaces (project_id, trello_id, name, url, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, trello_id)
		 DO UPDATE SET name = excluded.name, url = excluded.url
		 RETURNING id, project_id, trello_id, name, url, created_at`,
		pid, trelloID, name, url, now,
	).Scan(&w.ID, &w.ProjectID, &w.TrelloID, &w.Name, &w.URL, &w.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("attach trello workspace: %w", err)
	}
	return &w, nil
}

// DeleteTrelloWorkspace unlinks a workspace from the active project. Linked
// boards are removed by the ON DELETE CASCADE on trello_boards.workspace_id.
func (s *PostgresStore) DeleteTrelloWorkspace(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM trello_workspaces WHERE id = $1 AND project_id = $2`,
		id, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("delete trello workspace: %w", err)
	}
	return nil
}

// ListTrelloBoards returns the boards linked under a workspace (scoped to the
// active project so a caller cannot read another project's links), newest first.
func (s *PostgresStore) ListTrelloBoards(ctx context.Context, workspaceID int64) ([]TrelloBoardLink, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, workspace_id, trello_id, name, url, created_at
		 FROM trello_boards WHERE workspace_id = $1 AND project_id = $2
		 ORDER BY created_at DESC, id DESC`,
		workspaceID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list trello boards: %w", err)
	}
	defer rows.Close()
	var out []TrelloBoardLink
	for rows.Next() {
		var b TrelloBoardLink
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.WorkspaceID, &b.TrelloID, &b.Name, &b.URL, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan trello board: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AttachTrelloBoard links a Trello board under a workspace. The workspace must
// belong to the active project. It is an upsert on (project_id, trello_id), so a
// board can be moved between linked workspaces or have its name/url refreshed.
func (s *PostgresStore) AttachTrelloBoard(ctx context.Context, workspaceID int64, trelloID, name, url string) (*TrelloBoardLink, error) {
	pid := authctx.ProjectID(ctx)
	var owned bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM trello_workspaces WHERE id = $1 AND project_id = $2)`,
		workspaceID, pid).Scan(&owned); err != nil {
		return nil, fmt.Errorf("verify trello workspace: %w", err)
	}
	if !owned {
		return nil, ErrTrelloWorkspaceNotFound
	}
	now := time.Now().UTC()
	var b TrelloBoardLink
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trello_boards (project_id, workspace_id, trello_id, name, url, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (project_id, trello_id)
		 DO UPDATE SET workspace_id = excluded.workspace_id, name = excluded.name, url = excluded.url
		 RETURNING id, project_id, workspace_id, trello_id, name, url, created_at`,
		pid, workspaceID, trelloID, name, url, now,
	).Scan(&b.ID, &b.ProjectID, &b.WorkspaceID, &b.TrelloID, &b.Name, &b.URL, &b.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("attach trello board: %w", err)
	}
	return &b, nil
}

// DeleteTrelloBoard unlinks a board from the active project.
func (s *PostgresStore) DeleteTrelloBoard(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM trello_boards WHERE id = $1 AND project_id = $2`,
		id, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("delete trello board: %w", err)
	}
	return nil
}

// ErrTrelloWorkspaceNotFound is returned when attaching a board under a workspace
// that does not exist for the active project. Handlers map it to 404.
var ErrTrelloWorkspaceNotFound = errors.New("trello workspace not found")
