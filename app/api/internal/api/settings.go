package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
)

type llmSettingsUpdate struct {
	Provider *string `json:"provider"`
	// APIKey is optional; omit or send null to leave unchanged, empty string clears it.
	APIKey  *string `json:"api_key"`
	Model   *string `json:"model"`
	BaseURL *string `json:"base_url"`
	// Vision enables attaching inbound images to the chat request. Only turn on
	// for a vision-capable model; text-only models reject image content.
	Vision *bool `json:"vision"`
	// ResponseMode is "stream" (token-by-token SSE) or "block" (single response).
	ResponseMode *string `json:"response_mode"`
}

// handleSettings handles GET (masked view) and PUT (update) of LLM settings.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := s.settings.LLMView(r.Context())
		if err != nil {
			s.log.Error("failed to load settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
			return
		}
		writeJSON(w, http.StatusOK, view)

	case http.MethodPut:
		var req llmSettingsUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := s.settings.UpdateLLM(r.Context(), settings.LLMUpdate{
			Provider:     req.Provider,
			APIKey:       req.APIKey,
			Model:        req.Model,
			BaseURL:      req.BaseURL,
			Vision:       req.Vision,
			ResponseMode: req.ResponseMode,
		}); err != nil {
			s.log.Error("failed to update settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update settings"})
			return
		}
		view, err := s.settings.LLMView(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
			return
		}
		writeJSON(w, http.StatusOK, view)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSettingsTest makes a tiny live LLM call using the currently-saved
// settings to validate the API key/model/base URL.
func (s *Server) handleSettingsTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := s.settings.LLMConfig(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve settings"})
		return
	}
	if cfg.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no API key configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	_, err = s.llmClient.Complete(ctx, cfg, []llm.Message{
		{Role: "user", Content: "Reply with the single word: ok"},
	}, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "model": cfg.Model})
}
