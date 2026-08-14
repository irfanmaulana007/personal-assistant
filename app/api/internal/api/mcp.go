package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mcp"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// mcpClient is stateless (endpoint + auth are passed per call), so a single
// shared instance serves every request (mirrors trelloClient).
var mcpClient = mcp.NewClient()

// mcpServerResp is one MCP provider's per-project configuration + status.
type mcpServerResp struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Auth            string `json:"auth"` // "token" | "oauth"
	Mode            string `json:"mode"` // "read" | "readwrite"
	Endpoint        string `json:"endpoint"`
	DefaultEndpoint string `json:"default_endpoint"`
	SkillKey        string `json:"skill_key"`
	SkillEnabled    bool   `json:"skill_enabled"`
	// Token-auth providers (Cloudflare):
	Configured bool   `json:"configured,omitempty"`
	TokenMask  string `json:"token_mask,omitempty"`
	// OAuth providers (Notion, Railway):
	Connected bool `json:"connected,omitempty"`
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

// resolveEndpoint returns a provider's effective endpoint (settings override or
// the registry default) for the active project.
func (s *Server) resolveEndpoint(r *http.Request, info mcp.ProviderInfo) (string, string, error) {
	cfg, err := s.settings.MCPServer(r.Context(), string(info.Slug))
	if err != nil {
		return "", "", err
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = info.DefaultEndpoint
	}
	return endpoint, cfg.Token, nil
}

// handleListMCP returns each MCP provider's config/status for the active project
// plus the project's Notion database mapping.
func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	enabledKeys, err := s.store.EnabledProjectSkillKeys(r.Context(), authctx.ProjectID(r.Context()))
	if err != nil {
		s.log.Warn("read enabled skills", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	enabled := map[string]bool{}
	for _, k := range enabledKeys {
		enabled[k] = true
	}

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
		item := mcpServerResp{
			Slug:            string(info.Slug),
			Name:            info.Name,
			Auth:            string(info.Auth),
			Mode:            string(mcp.Mode(cfg.Mode).Normalize()),
			Endpoint:        endpoint,
			DefaultEndpoint: info.DefaultEndpoint,
			SkillKey:        info.SkillKey,
			SkillEnabled:    enabled[info.SkillKey],
		}
		if info.Auth == mcp.AuthOAuth {
			connected, err := s.mcpOAuth.Status(r.Context(), string(info.Slug))
			if err != nil {
				s.log.Warn("mcp oauth status", "provider", info.Slug, "error", err)
			}
			item.Connected = connected
		} else {
			item.Configured = cfg.Token != ""
			item.TokenMask = settings.Mask(cfg.Token)
		}
		resp.Servers = append(resp.Servers, item)
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

// handleSetMCPServer updates one provider's mode/endpoint (and token, for
// token-auth providers). A nil token leaves the stored one untouched; an empty
// string clears it. OAuth providers ignore the token.
func (s *Server) handleSetMCPServer(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported MCP provider"})
		return
	}
	var req struct {
		Mode     string  `json:"mode"`
		Endpoint string  `json:"endpoint"`
		Token    *string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == info.DefaultEndpoint {
		endpoint = "" // keep tracking the provider default
	}
	if err := s.settings.SetMCPServer(r.Context(), provider, req.Mode, endpoint); err != nil {
		s.log.Error("set mcp server", "provider", provider, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save configuration"})
		return
	}
	if info.Auth == mcp.AuthToken && req.Token != nil {
		if err := s.settings.SetMCPToken(r.Context(), provider, strings.TrimSpace(*req.Token)); err != nil {
			s.log.Error("set mcp token", "provider", provider, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save token"})
			return
		}
	}
	s.handleListMCP(w, r)
}

// handleTestMCP connects to a provider and lists its tools so an admin can verify
// the credential works. Token providers use the stored/supplied token; OAuth
// providers use the stored connection.
func (s *Server) handleTestMCP(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported MCP provider"})
		return
	}
	endpoint, storedToken, err := s.resolveEndpoint(r, info)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var auth mcp.Auth
	if info.Auth == mcp.AuthOAuth {
		ts, err := s.mcpOAuth.TokenSource(r.Context(), provider)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": info.Name + " is not connected — connect it first"})
			return
		}
		auth = mcp.OAuthAuth(ts)
	} else {
		var req struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		token := strings.TrimSpace(req.Token)
		if token == "" {
			token = storedToken
		}
		if token == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no token to test — enter one first"})
			return
		}
		auth = mcp.TokenAuth(token)
	}

	tools, err := mcpClient.ListTools(r.Context(), endpoint, auth)
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

// handleMCPOAuthConnect starts the OAuth flow for an OAuth-auth provider and
// returns the authorization URL for the browser to open.
func (s *Server) handleMCPOAuthConnect(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok || info.Auth != mcp.AuthOAuth {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not use OAuth"})
		return
	}
	endpoint, _, err := s.resolveEndpoint(r, info)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	authURL, err := s.mcpOAuth.Initiate(r.Context(), provider, endpoint, requestOrigin(r))
	if err != nil {
		s.log.Error("mcp oauth initiate", "provider", provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start the connection: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect_url": authURL})
}

// handleMCPOAuthCallback finishes the OAuth flow. It is UNAUTHENTICATED — the
// browser redirect carries no app session; the flow is validated by the opaque
// state. It renders a small self-closing page rather than redirecting into a
// project-scoped route (the connect UI opens this in a new tab).
func (s *Server) handleMCPOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	q := r.URL.Query()
	if errCode := q.Get("error"); errCode != "" {
		s.renderOAuthResult(w, provider, false, errCode)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		s.renderOAuthResult(w, provider, false, "missing state or code")
		return
	}
	if _, _, err := s.mcpOAuth.Complete(r.Context(), state, code); err != nil {
		s.log.Warn("mcp oauth complete", "provider", provider, "error", err)
		s.renderOAuthResult(w, provider, false, err.Error())
		return
	}
	s.renderOAuthResult(w, provider, true, "")
}

// handleMCPOAuthDisconnect removes an OAuth provider's connection for the project.
func (s *Server) handleMCPOAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	info, ok := mcp.Lookup(provider)
	if !ok || info.Auth != mcp.AuthOAuth {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not use OAuth"})
		return
	}
	if err := s.mcpOAuth.Disconnect(r.Context(), provider); err != nil {
		s.log.Error("mcp oauth disconnect", "provider", provider, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
		return
	}
	s.handleListMCP(w, r)
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

// requestOrigin returns the app's own public origin (scheme://host) as seen from
// behind a reverse proxy, used to build the OAuth callback URL. It uses the
// request's own host (not the frontend Origin header) because the callback route
// is served by this API.
func requestOrigin(r *http.Request) string {
	scheme := "https"
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if xh := r.Header.Get("X-Forwarded-Host"); xh != "" {
		host = xh
	}
	return scheme + "://" + host
}

// renderOAuthResult writes a minimal self-closing HTML page for the OAuth
// callback (opened in a new tab by the connect UI).
func (s *Server) renderOAuthResult(w http.ResponseWriter, provider string, ok bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	name := provider
	if info, ok := mcp.Lookup(provider); ok {
		name = info.Name
	}
	title, msg := "Connected", fmt.Sprintf("%s is now connected. You can close this tab and return to Personal Assistant.", name)
	if !ok {
		title, msg = "Connection failed", fmt.Sprintf("Could not connect %s: %s. You can close this tab and try again.", name, detail)
	}
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;background:#111827;color:#f9fafb;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
.card{max-width:26rem;padding:2rem;text-align:center}h1{font-size:1.25rem;margin:0 0 .5rem}p{color:#9ca3af;line-height:1.5}</style></head>
<body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, title, msg)
}
