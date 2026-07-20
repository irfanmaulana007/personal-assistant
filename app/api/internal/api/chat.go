package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

type chatRequest struct {
	Message string `json:"message"`
	// Image is an optional data: URL (base64) attached to the message, used by
	// vision skills such as Food Calories.
	Image string `json:"image"`
}

type chatResponse struct {
	Response string `json:"response"`
	// Images are data: URLs for any images the assistant generated this turn
	// (e.g. via the Image Generator skill). Omitted when there are none.
	Images []string `json:"images,omitempty"`
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

	if req.Message == "" && req.Image == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	claims := claimsFrom(r.Context())
	userID := int64(0)
	if claims != nil {
		userID = claims.UserID()
	}

	// A short text label for logs/history (the image itself is not stored).
	inputBody := req.Message
	if req.Image != "" {
		if inputBody == "" {
			inputBody = "[image]"
		} else {
			inputBody += " [image]"
		}
	}

	// Load recent conversation history for context (before logging the new message).
	history := s.recentHistory(r.Context(), userID)

	// Log incoming message
	_ = s.store.LogMessage(r.Context(), &store.MessageLog{
		UserID:    userID,
		Platform:  "web",
		Direction: "in",
		Sender:    "owner",
		Body:      inputBody,
	})

	// Stream mode (admin-configured) delivers the reply token-by-token over SSE;
	// otherwise fall through to the single-JSON-response path below.
	if s.settings.ResponseMode(r.Context()) == "stream" {
		if _, ok := w.(http.Flusher); ok {
			s.streamChat(w, r, userID, inputBody, req, history)
			return
		}
		// No flusher (unusual) — degrade gracefully to the blocking path.
	}

	// Run the LLM agent.
	start := time.Now()
	res, err := s.agent.Run(r.Context(), req.Message, history, req.Image)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		if errors.Is(err, agent.ErrNotConfigured) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "The assistant isn't configured yet. Add your LLM API key in Settings to start chatting.",
			})
			return
		}
		s.log.Error("agent run failed", "error", err)
		_, _ = s.store.CreateTrace(r.Context(), &store.Trace{
			UserID:    userID,
			Platform:  "web",
			Input:     inputBody,
			LatencyMs: latencyMs,
			Status:    "error",
			Error:     err.Error(),
		})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "The assistant ran into a problem talking to the LLM. Check your Settings and try again."})
		return
	}

	images := s.recordChatOutcome(r.Context(), userID, inputBody, res, latencyMs)
	writeJSON(w, http.StatusOK, chatResponse{Response: res.Reply, Images: images})
}

// streamChat runs the agent with token-by-token streaming and emits the reply
// as Server-Sent Events. Frames:
//   - {"type":"delta","text":"…"}      incremental reply text
//   - {"type":"done","response":"…","images":[…]}  final authoritative reply
//   - {"type":"error","error":"…"}     failure (also ends the stream)
//
// All logging/tracing happens on completion via recordChatOutcome, identically
// to the blocking path — streaming only changes delivery, not bookkeeping.
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, userID int64, inputBody string, req chatRequest, history []agent.Message) {
	flusher := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // ask proxies (nginx) not to buffer
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sendEvent := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	start := time.Now()
	res, err := s.agent.RunStream(r.Context(), req.Message, history, req.Image, func(delta string) {
		sendEvent(map[string]string{"type": "delta", "text": delta})
	})
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		msg := "The assistant ran into a problem talking to the LLM. Check your Settings and try again."
		if errors.Is(err, agent.ErrNotConfigured) {
			msg = "The assistant isn't configured yet. Add your LLM API key in Settings to start chatting."
		} else {
			s.log.Error("agent stream failed", "error", err)
		}
		_, _ = s.store.CreateTrace(r.Context(), &store.Trace{
			UserID:    userID,
			Platform:  "web",
			Input:     inputBody,
			LatencyMs: latencyMs,
			Status:    "error",
			Error:     err.Error(),
		})
		sendEvent(map[string]string{"type": "error", "error": msg})
		return
	}

	images := s.recordChatOutcome(r.Context(), userID, inputBody, res, latencyMs)
	sendEvent(map[string]any{"type": "done", "response": res.Reply, "images": images})
}

// recordChatOutcome logs the assistant reply, writes the full trace, records
// per-tool usage, and kicks off inline eval sampling. It returns the reply's
// image data URLs (never nil) for the response. Shared by the blocking and
// streaming paths so their bookkeeping stays identical.
func (s *Server) recordChatOutcome(ctx context.Context, userID int64, inputBody string, res *agent.Result, latencyMs int) []string {
	// Log outgoing message (chat history)
	_ = s.store.LogMessage(ctx, &store.MessageLog{
		UserID:    userID,
		Platform:  "web",
		Direction: "out",
		Sender:    "assistant",
		Body:      res.Reply,
		Intent:    "agent",
		Action:    res.Model,
	})

	// Record the full trace (dashboard + logs) and per-tool usage.
	traceID, _ := s.store.CreateTrace(ctx, &store.Trace{
		UserID:                userID,
		Platform:              "web",
		Input:                 inputBody,
		Output:                res.Reply,
		Model:                 res.Model,
		PromptTokens:          res.Usage.PromptTokens,
		CompletionTokens:      res.Usage.CompletionTokens,
		TotalTokens:           res.Usage.TotalTokens,
		ImageModel:            res.ImageModel,
		ImagePromptTokens:     res.ImagePromptTokens,
		ImageCompletionTokens: res.ImageCompletionTokens,
		ImageTotalTokens:      res.ImageTotalTokens,
		LatencyMs:             latencyMs,
		ToolCount:             len(res.Tools),
		Tools:                 toStoreTools(res.Tools),
		Steps:                 toStoreSteps(res.Steps),
		Skills:                res.Skills,
		Status:                "ok",
	})
	for _, tool := range res.Tools {
		_ = s.store.LogToolUsage(ctx, userID, tool.Name, "web")
	}
	// Judge a sampled fraction of live replies out of band (never blocks).
	if s.eval != nil {
		s.eval.JudgeInline(ctx, traceID)
	}

	images := make([]string, 0, len(res.Images))
	for _, img := range res.Images {
		images = append(images, img.DataURL())
	}
	return images
}

// toStoreTools converts agent tool invocations to the store representation,
// carrying the per-tool image usage (model + tokens) so it can be priced in the
// logs detail apart from the LLM.
func toStoreTools(inv []agent.ToolInvocation) []store.ToolInvocation {
	out := make([]store.ToolInvocation, len(inv))
	for i, t := range inv {
		out[i] = store.ToolInvocation{
			Name: t.Name, Arguments: t.Arguments, Result: t.Result, LatencyMs: t.LatencyMs,
			Model: t.Model, PromptTokens: t.PromptTokens,
			CompletionTokens: t.CompletionTokens, TotalTokens: t.TotalTokens,
		}
	}
	return out
}

// toStoreSteps converts agent LLM-call records to the store representation.
func toStoreSteps(steps []agent.LLMCall) []store.LLMCall {
	out := make([]store.LLMCall, len(steps))
	for i, s := range steps {
		out[i] = store.LLMCall{
			Step: s.Step, Model: s.Model, PromptTokens: s.PromptTokens,
			CompletionTokens: s.CompletionTokens, TotalTokens: s.TotalTokens,
			LatencyMs: s.LatencyMs, FinishReason: s.FinishReason, ToolCalls: s.ToolCalls,
		}
	}
	return out
}

// recentHistory returns the last few web turns as agent context (oldest first).
func (s *Server) recentHistory(ctx context.Context, userID int64) []agent.Message {
	const maxTurns = 20
	logs, err := s.store.GetMessageHistory(ctx, userID, "web", maxTurns)
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

	userID := int64(0)
	if claims := claimsFrom(r.Context()); claims != nil {
		userID = claims.UserID()
	}
	logs, err := s.store.GetMessageHistory(r.Context(), userID, "web", 100)
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
