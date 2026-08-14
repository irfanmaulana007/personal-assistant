package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mcp"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// mcpClient is stateless (endpoint + token are passed per call), so a single
// shared instance serves every request (mirrors trelloClient).
var mcpClient = mcp.NewClient()

// mcpServerResp is one MCP provider's per-project configuration + status.
type mcpServerResp struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"` // "read" | "readwrite"
	Endpoint        string `json:"endpoint"`
	DefaultEndpoint string `json:"default_endpoint"`
	Configured      bool   `json:"configured"` // a token is stored
	TokenMask       string `json:"token_mask"`
	// OAuthCaveat marks providers whose hosted MCP is OAuth-first (Railway), so
	// the UI can warn that a static token may only work via a proxy.
	OAuthCaveat bool `json:"oauth_caveat,omitempty"`
}

type notionTargetResp struct {
	Kind       string `json:"kind"`
	DatabaseID string `json:"database_id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
}

type mcpResp struct {
	Servers       []mcpServerResp    `json:"servers"`
	NotionTargets []notionTargetResp `json:"notion_targets"`
}

func toNotionTargetResp(t store.NotionTarget) notionTargetResp {
	return notionTargetResp{Kind: t.Kind, DatabaseID: t.DatabaseID, Name: t.Name, URL: t.URL}
}

// handleListMCP returns each MCP provider's config/status for the active project
// plus the project's Notion database mapping.
func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	resp := mcpResp{Servers: make([]mcpServerResp, 0, len(mcp.Registry()))}
	for _, info := range mcp.Registry() {
		cfg, err := s.settings.MCPServer(r.Context(), string(info.Slug))
		if err != nil {
			s.log.Warn("read mcp config", "provider", info.Slug, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = info.DefaultEndpoint
		}
		mode := string(mcp.Mode(cfg.Mode).Normalize())
		resp.Servers = append(resp.Servers, mcpServerResp{
			Slug:            string(info.Slug),
			Name:            info.Name,
			Enabled:         cfg.Enabled,
			Mode:            mode,
			Endpoint:        endpoint,
			DefaultEndpoint: info.DefaultEndpoint,
			Configured:      cfg.Token != "",
			TokenMask:       settings.Mask(cfg.Token),
			OAuthCaveat:     info.Slug == mcp.Railway,
		})
	}

	targets, err := s.store.ListNotionTargets(r.Context())
	if err != nil {
		s.log.Warn("list notion targets", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load Notion mapping"})
		return
	}
	resp.NotionTargets = make([]notionTargetResp, 0, len(targets))
	for _, t := range targets {
		resp.NotionTargets = append(resp.NotionTargets, toNotionTargetResp(t))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSetMCPServer updates one provider's config. The token is optional: a nil
// token leaves the stored one untouched; an empty string clears it.
func (s *Server) handleSetMCPServer(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported MCP provider"})
		return
	}
	var req struct {
		Enabled  bool    `json:"enabled"`
		Mode     string  `json:"mode"`
		Endpoint string  `json:"endpoint"`
		Token    *string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	// Persisting the default endpoint as "" keeps a project tracking the provider
	// default if we ever change it.
	if endpoint == info.DefaultEndpoint {
		endpoint = ""
	}
	if err := s.settings.SetMCPServer(r.Context(), provider, req.Enabled, req.Mode, endpoint); err != nil {
		s.log.Error("set mcp server", "provider", provider, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save configuration"})
		return
	}
	if req.Token != nil {
		if err := s.settings.SetMCPToken(r.Context(), provider, strings.TrimSpace(*req.Token)); err != nil {
			s.log.Error("set mcp token", "provider", provider, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save token"})
			return
		}
	}
	s.handleListMCP(w, r)
}

// handleTestMCP connects to a provider and lists its tools so an admin can
// verify a token before relying on it. A token in the body is used when present
// (test-before-save); otherwise the stored token is used.
func (s *Server) handleTestMCP(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported MCP provider"})
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
		Token    string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	endpoint := strings.TrimSpace(req.Endpoint)
	token := strings.TrimSpace(req.Token)
	if endpoint == "" || token == "" {
		cfg, err := s.settings.MCPServer(r.Context(), provider)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if endpoint == "" {
			endpoint = cfg.Endpoint
		}
		if token == "" {
			token = cfg.Token
		}
	}
	if endpoint == "" {
		endpoint = info.DefaultEndpoint
	}
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no token to test — enter one first"})
		return
	}

	tools, err := mcpClient.ListTools(r.Context(), endpoint, token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tool_count": len(tools), "tools": names})
}

// handleSetNotionTarget maps (upserts) a Notion database to the active project
// under a label ("task", "issue", …).
func (s *Server) handleSetNotionTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind       string `json:"kind"`
		DatabaseID string `json:"database_id"`
		Name       string `json:"name"`
		URL        string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	kind := strings.TrimSpace(req.Kind)
	databaseID := strings.TrimSpace(req.DatabaseID)
	if kind == "" || databaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind and database_id are required"})
		return
	}
	if _, err := s.store.SetNotionTarget(r.Context(), kind, databaseID, strings.TrimSpace(req.Name), strings.TrimSpace(req.URL)); err != nil {
		s.log.Error("set notion target", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save mapping"})
		return
	}
	s.handleListMCP(w, r)
}

// handleDeleteNotionTarget removes a Notion database mapping by label.
func (s *Server) handleDeleteNotionTarget(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.PathValue("kind"))
	if kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind is required"})
		return
	}
	if err := s.store.DeleteNotionTarget(r.Context(), kind); err != nil {
		s.log.Error("delete notion target", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove mapping"})
		return
	}
	s.handleListMCP(w, r)
}
