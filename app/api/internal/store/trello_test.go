//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
)

func TestTrelloLinksCRUDRoundTrip(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	// Attach a workspace.
	ws, err := s.AttachTrelloWorkspace(ctx, "org1", "Acme Inc", "https://trello.com/acme")
	if err != nil {
		t.Fatalf("attach workspace: %v", err)
	}
	if ws.ID == 0 || ws.TrelloID != "org1" || ws.Name != "Acme Inc" {
		t.Fatalf("unexpected workspace: %+v", ws)
	}

	// Re-attaching the same Trello id upserts (refreshes name), not duplicates.
	ws2, err := s.AttachTrelloWorkspace(ctx, "org1", "Acme Renamed", "https://trello.com/acme")
	if err != nil {
		t.Fatalf("re-attach workspace: %v", err)
	}
	if ws2.ID != ws.ID || ws2.Name != "Acme Renamed" {
		t.Fatalf("expected upsert to same row with new name: %+v", ws2)
	}
	if list, err := s.ListTrelloWorkspaces(ctx); err != nil || len(list) != 1 {
		t.Fatalf("list workspaces = %+v (err %v), want 1", list, err)
	}

	// Attach two boards under it.
	b1, err := s.AttachTrelloBoard(ctx, ws.ID, "board1", "Roadmap", "https://trello.com/b/board1")
	if err != nil {
		t.Fatalf("attach board1: %v", err)
	}
	if _, err := s.AttachTrelloBoard(ctx, ws.ID, "board2", "Bugs", "https://trello.com/b/board2"); err != nil {
		t.Fatalf("attach board2: %v", err)
	}
	boards, err := s.ListTrelloBoards(ctx, ws.ID)
	if err != nil || len(boards) != 2 {
		t.Fatalf("list boards = %+v (err %v), want 2", boards, err)
	}

	// Attaching under a non-existent workspace is rejected.
	if _, err := s.AttachTrelloBoard(ctx, 99999, "boardX", "X", ""); !errors.Is(err, ErrTrelloWorkspaceNotFound) {
		t.Fatalf("attach board to missing workspace err = %v, want ErrTrelloWorkspaceNotFound", err)
	}

	// Delete one board.
	if err := s.DeleteTrelloBoard(ctx, b1.ID); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	if boards, _ := s.ListTrelloBoards(ctx, ws.ID); len(boards) != 1 {
		t.Fatalf("after delete, boards = %d, want 1", len(boards))
	}

	// Deleting the workspace cascades to its remaining boards.
	if err := s.DeleteTrelloWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if list, _ := s.ListTrelloWorkspaces(ctx); len(list) != 0 {
		t.Fatalf("after delete, workspaces = %d, want 0", len(list))
	}
	if boards, _ := s.ListTrelloBoards(ctx, ws.ID); len(boards) != 0 {
		t.Fatalf("cascade failed, boards = %d, want 0", len(boards))
	}
}
