package api

import (
	"encoding/json"
	"net/http"

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

	// Log incoming message
	_ = s.store.LogMessage(r.Context(), &store.MessageLog{
		Platform:  "web",
		Direction: "in",
		Sender:    "owner",
		Body:      req.Message,
	})

	// Parse intent
	result := s.parser.Parse(req.Message)

	// Route to handler
	response := s.router.Route(r.Context(), result)

	// Log outgoing message
	_ = s.store.LogMessage(r.Context(), &store.MessageLog{
		Platform:  "web",
		Direction: "out",
		Sender:    "assistant",
		Body:      response,
		Intent:    string(result.Capability),
		Action:    string(result.Action),
	})

	writeJSON(w, http.StatusOK, chatResponse{Response: response})
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
