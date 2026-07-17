// Package memory implements the always-on memory capability: the agent's
// remember/recall tools over the per-user memory store.
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/memory"
)

// Handler saves and recalls durable memories for the user.
type Handler struct {
	mem *memory.Service
	log *slog.Logger
}

// New creates a memory capability handler over the shared memory service.
func New(mem *memory.Service, log *slog.Logger) *Handler {
	return &Handler{mem: mem, log: log.With("component", "memory")}
}

func (h *Handler) Name() string { return "memory" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityMemory
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionMemoryRemember:
		return h.remember(ctx, result)
	case intent.ActionMemoryRecall:
		return h.recall(ctx, result)
	default:
		return "I can remember a fact or recall what I know.", nil
	}
}

func (h *Handler) remember(ctx context.Context, result *intent.ParseResult) (string, error) {
	content := strings.TrimSpace(result.Entities["content"])
	if content == "" {
		return "What should I remember?", nil
	}
	userID := authctx.UserID(ctx)
	saved, err := h.mem.Save(ctx, userID, content)
	if err != nil {
		return "", fmt.Errorf("save memory: %w", err)
	}

	// Read-after-write: confirm the memory actually persisted before telling the
	// user it was remembered.
	got, err := h.mem.Get(ctx, userID, saved.ID)
	if err != nil {
		return "", fmt.Errorf("verify memory saved: %w", err)
	}
	if got == nil {
		return "", fmt.Errorf("verify memory saved: memory not found after create")
	}
	return fmt.Sprintf("Got it — I'll remember that: %s", got.Content), nil
}

func (h *Handler) recall(ctx context.Context, result *intent.ParseResult) (string, error) {
	query := strings.TrimSpace(result.Entities["query"])
	mems, err := h.mem.Search(ctx, authctx.UserID(ctx), query, 8)
	if err != nil {
		return "", fmt.Errorf("recall memory: %w", err)
	}
	if len(mems) == 0 {
		return "I don't have anything remembered about that yet.", nil
	}
	var sb strings.Builder
	sb.WriteString("Here's what I remember:")
	for _, m := range mems {
		sb.WriteString("\n• " + m.Content)
	}
	return sb.String(), nil
}
