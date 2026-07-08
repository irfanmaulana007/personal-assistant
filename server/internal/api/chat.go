package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/irfanmaulana007/personal-assistant/server/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Response string `json:"response"`
}

type historyEntry struct {
	Direction string `json:"direction"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Load recent conversation history for context (before logging the new message).
	history := s.recentHistory(r.Context())

	// Log incoming message
	_ = s.store.LogMessage(r.Context(), &store.MessageLog{
		Platform:  "web",
		Direction: "in",
		Sender:    "owner",
		Body:      req.Message,
	})

	// Run the LLM agent.
	res, err := s.agent.Run(r.Context(), req.Message, history)
	if err != nil {
		if errors.Is(err, agent.ErrNotConfigured) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "The assistant isn't configured yet. Add your LLM API key in Settings to start chatting.",
			})
			return
		}
		s.log.Error("agent run failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "The assistant ran into a problem talking to the LLM. Check your Settings and try again."})
		return
	}

	// Log outgoing message
	_ = s.store.LogMessage(r.Context(), &store.MessageLog{
		Platform:  "web",
		Direction: "out",
		Sender:    "assistant",
		Body:      res.Reply,
		Intent:    "agent",
		Action:    res.Model,
	})

	// Record token usage for the dashboard.
	_ = s.store.LogUsage(r.Context(), &store.LLMUsage{
		Model:            res.Model,
		PromptTokens:     res.Usage.PromptTokens,
		CompletionTokens: res.Usage.CompletionTokens,
		TotalTokens:      res.Usage.TotalTokens,
		Platform:         "web",
	})

	writeJSON(w, http.StatusOK, chatResponse{Response: res.Reply})
}

// recentHistory returns the last few web turns as agent context (oldest first).
func (s *Server) recentHistory(ctx context.Context) []agent.Message {
	const maxTurns = 10
	logs, err := s.store.GetMessageHistory(ctx, "web", maxTurns)
	if err != nil {
		s.log.Warn("failed to load history for agent context", "error", err)
		return nil
	}
	history := make([]agent.Message, 0, len(logs))
	for _, l := range logs {
		role := "assistant"
		if l.Direction == "in" {
			role = "user"
		}
		if l.Body == "" {
			continue
		}
		history = append(history, agent.Message{Role: role, Content: l.Body})
	}
	return history
}

func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logs, err := s.store.GetMessageHistory(r.Context(), "web", 100)
	if err != nil {
		s.log.Error("failed to get chat history", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load history"})
		return
	}

	entries := make([]historyEntry, len(logs))
	for i, l := range logs {
		entries[i] = historyEntry{
			Direction: l.Direction,
			Body:      l.Body,
			Timestamp: l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, entries)
}
