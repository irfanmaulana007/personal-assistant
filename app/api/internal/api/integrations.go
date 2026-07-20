package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
)

// supportedToolkits are the Composio apps this app exposes on the Integrations
// page. Slugs are Composio toolkit slugs.
var supportedToolkits = []struct {
	Slug string
	Name string
}{
	{"gmail", "Gmail"},
	{"googlecalendar", "Google Calendar"},
	{"github", "GitHub"},
	{"sentry", "Sentry"},
	{"trello", "Trello"},
}

func toolkitName(slug string) (string, bool) {
	for _, t := range supportedToolkits {
		if t.Slug == slug {
			return t.Name, true
		}
	}
	return "", false
}

// multiToolkits may have several connected accounts per user (e.g. multiple
// Google accounts). Other toolkits are treated as a single connection.
var multiToolkits = map[string]bool{"googlecalendar": true, "gmail": true}

type integrationAccount struct {
	ConnectionID string `json:"connection_id"`
	Status       string `json:"status"`
	Label        string `json:"label,omitempty"` // the account's email, when known
}

type integrationToolkit struct {
	Slug         string               `json:"slug"`
	Name         string               `json:"name"`
	Status       string               `json:"status"` // connected, pending, error, disconnected
	ConnectionID string               `json:"connection_id,omitempty"`
	Multi        bool                 `json:"multi,omitempty"`
	Accounts     []integrationAccount `json:"accounts,omitempty"`
}

type integrationsResp struct {
	Configured bool                 `json:"configured"`
	APIKeyMask string               `json:"api_key_mask"`
	Toolkits   []integrationToolkit `json:"toolkits"`
	// Web search (Tavily) is a standalone API key, independent of Composio.
	WebSearchConfigured bool   `json:"web_search_configured"`
	WebSearchKeyMask    string `json:"web_search_key_mask"`
	// OpenAI key powers the Image Generator skill (gpt-image-1).
	OpenAIConfigured bool   `json:"openai_configured"`
	OpenAIKeyMask    string `json:"openai_key_mask"`
	// Trello key + token power the Trello Board Review and Card Creator skills.
	TrelloConfigured bool   `json:"trello_configured"`
	TrelloKeyMask    string `json:"trello_key_mask"`
	TrelloTokenMask  string `json:"trello_token_mask"`
}

func statusFromComposio(s string) string {
	switch strings.ToUpper(s) {
	case "ACTIVE":
		return "connected"
	case "INITIATED":
		return "pending"
	case "FAILED":
		return "error"
	default:
		return "disconnected"
	}
}

func (s *Server) composioUserID(r *http.Request) string {
	if claims := claimsFrom(r.Context()); claims != nil {
		return strconv.FormatInt(claims.UserID(), 10)
	}
	return ""
}

// handleListIntegrations returns the Composio key status and each toolkit's
// connection status for the current user.
func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	key, err := s.settings.ComposioKey(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	resp := integrationsResp{
		Configured: key != "",
		APIKeyMask: settings.Mask(key),
		Toolkits:   make([]integrationToolkit, 0, len(supportedToolkits)),
	}

	// Web search key status (independent of Composio).
	if wsKey, err := s.settings.WebSearchKey(r.Context()); err == nil {
		resp.WebSearchConfigured = wsKey != ""
		resp.WebSearchKeyMask = settings.Mask(wsKey)
	} else {
		s.log.Warn("resolve web search key", "error", err)
	}

	// OpenAI key status (image generation; independent of Composio).
	if oaKey, err := s.settings.OpenAIKey(r.Context()); err == nil {
		resp.OpenAIConfigured = oaKey != ""
		resp.OpenAIKeyMask = settings.Mask(oaKey)
	} else {
		s.log.Warn("resolve openai key", "error", err)
	}

	// Trello credentials status (board review + card creation; independent of
	// Composio). Both the key and the token must be present to be "configured".
	if tKey, tToken, err := s.settings.TrelloCreds(r.Context()); err == nil {
		resp.TrelloConfigured = tKey != "" && tToken != ""
		resp.TrelloKeyMask = settings.Mask(tKey)
		resp.TrelloTokenMask = settings.Mask(tToken)
	} else {
		s.log.Warn("resolve trello creds", "error", err)
	}

	// Gather all connections per toolkit slug (a user may have several).
	accountsBySlug := map[string][]integrationAccount{}
	if key != "" {
		conns, err := s.composio.ListConnections(r.Context(), key, s.composioUserID(r))
		if err != nil {
			s.log.Warn("composio list connections failed", "error", err)
		} else {
			for _, c := range conns {
				accountsBySlug[c.ToolkitSlug] = append(accountsBySlug[c.ToolkitSlug], integrationAccount{
					ConnectionID: c.ID,
					Status:       statusFromComposio(c.Status),
					Label:        c.Label,
				})
			}
		}
	}

	for _, t := range supportedToolkits {
		item := integrationToolkit{Slug: t.Slug, Name: t.Name, Status: "disconnected"}
		accts := accountsBySlug[t.Slug]
		if multiToolkits[t.Slug] {
			item.Multi = true
			item.Accounts = accts
			for _, a := range accts { // header reflects the best account state
				if a.Status == "connected" {
					item.Status = "connected"
					break
				}
				if a.Status == "pending" {
					item.Status = "pending"
				}
			}
		} else if len(accts) > 0 {
			// Single-connection toolkit: use the first.
			item.Status = accts[0].Status
			item.ConnectionID = accts[0].ConnectionID
		}
		resp.Toolkits = append(resp.Toolkits, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSetComposioKey stores/clears the Composio API key.
func (s *Server) handleSetComposioKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.settings.SetComposioKey(r.Context(), strings.TrimSpace(req.APIKey)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
		return
	}
	s.handleListIntegrations(w, r)
}

// handleSetWebSearchKey stores/clears the web-search (Tavily) API key.
func (s *Server) handleSetWebSearchKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.settings.SetWebSearchKey(r.Context(), strings.TrimSpace(req.APIKey)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
		return
	}
	s.handleListIntegrations(w, r)
}

// handleSetOpenAIKey stores/clears the OpenAI API key (image generation).
func (s *Server) handleSetOpenAIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.settings.SetOpenAIKey(r.Context(), strings.TrimSpace(req.APIKey)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
		return
	}
	s.handleListIntegrations(w, r)
}

// handleSetTrelloCreds stores/clears the Trello API key and user token.
func (s *Server) handleSetTrelloCreds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.settings.SetTrelloCreds(r.Context(), strings.TrimSpace(req.APIKey), strings.TrimSpace(req.Token)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save credentials"})
		return
	}
	s.handleListIntegrations(w, r)
}

// handleConnectIntegration starts an OAuth connection and returns the redirect URL.
func (s *Server) handleConnectIntegration(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("toolkit")
	if _, ok := toolkitName(slug); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported toolkit"})
		return
	}

	key, err := s.settings.ComposioKey(r.Context())
	if err != nil || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Composio API key not configured"})
		return
	}

	authConfigID, err := s.composio.EnsureAuthConfig(r.Context(), key, slug)
	if err != nil {
		s.log.Error("composio ensure auth config", "toolkit", slug, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not prepare the connection: " + err.Error()})
		return
	}

	callback := ""
	if origin := r.Header.Get("Origin"); origin != "" {
		callback = origin + "/integrations"
	}

	redirectURL, _, err := s.composio.InitiateConnection(r.Context(), key, authConfigID, s.composioUserID(r), callback)
	if err != nil {
		s.log.Error("composio initiate connection", "toolkit", slug, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start the connection: " + err.Error()})
		return
	}
	if redirectURL == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Composio did not return an authorization URL"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirectURL})
}

// handleDisconnectIntegration removes the current user's connection for a toolkit.
func (s *Server) handleDisconnectIntegration(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("toolkit")
	if _, ok := toolkitName(slug); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported toolkit"})
		return
	}

	key, err := s.settings.ComposioKey(r.Context())
	if err != nil || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Composio API key not configured"})
		return
	}

	// Optionally target a single account (multi-connection toolkits); otherwise
	// remove every connection for the toolkit.
	only := strings.TrimSpace(r.URL.Query().Get("connection_id"))

	conns, err := s.composio.ListConnections(r.Context(), key, s.composioUserID(r))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	for _, c := range conns {
		if c.ToolkitSlug != slug {
			continue
		}
		if only != "" && c.ID != only {
			continue
		}
		if err := s.composio.DeleteConnection(r.Context(), key, c.ID); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}
	s.handleListIntegrations(w, r)
}
