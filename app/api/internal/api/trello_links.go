package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/trello"
)

// trelloClient is stateless (auth is passed per call) and safe for concurrent
// use, so a single shared instance serves every request.
var trelloClient = trello.New()

// Linked (persisted) workspace/board wire shapes. ID is our DB row id; TrelloID
// is the Trello object id.
type trelloWorkspaceResp struct {
	ID       int64  `json:"id"`
	TrelloID string `json:"trello_id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type trelloBoardResp struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	TrelloID    string `json:"trello_id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
}

// Remote (live-from-Trello) shapes for the pickers. ID is the Trello id.
type trelloRemoteWorkspaceResp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
}

type trelloRemoteBoardResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type trelloAttachWorkspaceReq struct {
	TrelloID string `json:"trello_id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type trelloAttachBoardReq struct {
	TrelloID string `json:"trello_id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

func toTrelloWorkspaceResp(w store.TrelloWorkspaceLink) trelloWorkspaceResp {
	return trelloWorkspaceResp{ID: w.ID, TrelloID: w.TrelloID, Name: w.Name, URL: w.URL}
}

func toTrelloBoardResp(b store.TrelloBoardLink) trelloBoardResp {
	return trelloBoardResp{ID: b.ID, WorkspaceID: b.WorkspaceID, TrelloID: b.TrelloID, Name: b.Name, URL: b.URL}
}

// trelloCreds resolves the active project's Trello credentials, writing a 400 and
// returning ok=false when they are not configured (the live pickers need them).
func (s *Server) trelloCreds(w http.ResponseWriter, r *http.Request) (apiKey, token string, ok bool) {
	apiKey, token, err := s.settings.TrelloCreds(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read Trello credentials"})
		return "", "", false
	}
	if apiKey == "" || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Trello credentials are not configured"})
		return "", "", false
	}
	return apiKey, token, true
}

// handleTrelloAvailableWorkspaces lists the workspaces the token can see, so the
// user can pick which ones to link to this project.
func (s *Server) handleTrelloAvailableWorkspaces(w http.ResponseWriter, r *http.Request) {
	apiKey, token, ok := s.trelloCreds(w, r)
	if !ok {
		return
	}
	orgs, err := trelloClient.Organizations(r.Context(), apiKey, token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	out := make([]trelloRemoteWorkspaceResp, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, trelloRemoteWorkspaceResp{ID: o.ID, Name: o.Name, DisplayName: o.DisplayName, URL: o.URL})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListTrelloWorkspaces returns the workspaces linked to this project.
func (s *Server) handleListTrelloWorkspaces(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTrelloWorkspaces(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load Trello workspaces"})
		return
	}
	out := make([]trelloWorkspaceResp, 0, len(items))
	for _, it := range items {
		out = append(out, toTrelloWorkspaceResp(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAttachTrelloWorkspace links a Trello workspace to this project.
func (s *Server) handleAttachTrelloWorkspace(w http.ResponseWriter, r *http.Request) {
	var req trelloAttachWorkspaceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	trelloID := strings.TrimSpace(req.TrelloID)
	if trelloID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trello_id is required"})
		return
	}
	ws, err := s.store.AttachTrelloWorkspace(r.Context(), trelloID, strings.TrimSpace(req.Name), strings.TrimSpace(req.URL))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to link workspace"})
		return
	}
	writeJSON(w, http.StatusOK, toTrelloWorkspaceResp(*ws))
}

// handleDeleteTrelloWorkspace unlinks a workspace (and its boards) from this project.
func (s *Server) handleDeleteTrelloWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteTrelloWorkspace(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unlink workspace"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// linkedWorkspaceTrelloID resolves a linked workspace's Trello org id from its DB
// id, scoped to the active project. Returns ok=false (no write) when not found so
// the caller can 404.
func (s *Server) linkedWorkspaceTrelloID(r *http.Request, id int64) (trelloID string, ok bool, err error) {
	items, err := s.store.ListTrelloWorkspaces(r.Context())
	if err != nil {
		return "", false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it.TrelloID, true, nil
		}
	}
	return "", false, nil
}

// handleTrelloAvailableBoards lists the boards in a linked workspace, so the user
// can pick which to link under it.
func (s *Server) handleTrelloAvailableBoards(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	orgID, ok, err := s.linkedWorkspaceTrelloID(r, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load workspace"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
		return
	}
	apiKey, token, credsOK := s.trelloCreds(w, r)
	if !credsOK {
		return
	}
	boards, err := trelloClient.OrganizationBoards(r.Context(), apiKey, token, orgID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	out := make([]trelloRemoteBoardResp, 0, len(boards))
	for _, b := range boards {
		out = append(out, trelloRemoteBoardResp{ID: b.ID, Name: b.Name, URL: b.URL})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListTrelloBoards returns the boards linked under a workspace.
func (s *Server) handleListTrelloBoards(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	items, err := s.store.ListTrelloBoards(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load Trello boards"})
		return
	}
	out := make([]trelloBoardResp, 0, len(items))
	for _, it := range items {
		out = append(out, toTrelloBoardResp(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAttachTrelloBoard links a Trello board under a linked workspace.
func (s *Server) handleAttachTrelloBoard(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req trelloAttachBoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	trelloID := strings.TrimSpace(req.TrelloID)
	if trelloID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trello_id is required"})
		return
	}
	b, err := s.store.AttachTrelloBoard(r.Context(), workspaceID, trelloID, strings.TrimSpace(req.Name), strings.TrimSpace(req.URL))
	if errors.Is(err, store.ErrTrelloWorkspaceNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to link board"})
		return
	}
	writeJSON(w, http.StatusOK, toTrelloBoardResp(*b))
}

// handleDeleteTrelloBoard unlinks a board from this project.
func (s *Server) handleDeleteTrelloBoard(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteTrelloBoard(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unlink board"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
