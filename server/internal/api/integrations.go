package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
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
}

func toolkitName(slug string) (string, bool) {
	for _, t := range supportedToolkits {
		if t.Slug == slug {
			return t.Name, true
		}
	}
	return "", false
}

type integrationToolkit struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Status       string `json:"status"` // connected, pending, error, disconnected
	ConnectionID string `json:"connection_id,omitempty"`
}

type integrationsResp struct {
	Configured bool                 `json:"configured"`
	APIKeyMask string               `json:"api_key_mask"`
	Toolkits   []integrationToolkit `json:"toolkits"`
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

	// Map connection status by toolkit slug when configured.
	statusBySlug := map[string]struct {
		status string
		id     string
	}{}
	if key != "" {
		conns, err := s.composio.ListConnections(r.Context(), key, s.composioUserID(r))
		if err != nil {
			s.log.Warn("composio list connections failed", "error", err)
		} else {
			for _, c := range conns {
				// Keep the first/most relevant connection per toolkit.
				if _, ok := statusBySlug[c.ToolkitSlug]; !ok {
					statusBySlug[c.ToolkitSlug] = struct {
						status string
						id     string
					}{statusFromComposio(c.Status), c.ID}
				}
			}
		}
	}

	for _, t := range supportedToolkits {
		item := integrationToolkit{Slug: t.Slug, Name: t.Name, Status: "disconnected"}
		if st, ok := statusBySlug[t.Slug]; ok {
			item.Status = st.status
			item.ConnectionID = st.id
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

	conns, err := s.composio.ListConnections(r.Context(), key, s.composioUserID(r))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	for _, c := range conns {
		if c.ToolkitSlug == slug {
			if err := s.composio.DeleteConnection(r.Context(), key, c.ID); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	s.handleListIntegrations(w, r)
}
