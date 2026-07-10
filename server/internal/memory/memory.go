// Package memory is the agent's long-term memory: durable facts persisted per
// user and retrieved via SQLite FTS5.
package memory

import (
	"context"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Service saves and retrieves user memories.
type Service struct {
	store store.Store
}

// New creates a memory service.
func New(s store.Store) *Service {
	return &Service{store: s}
}

// Save persists a durable fact for the user.
func (s *Service) Save(ctx context.Context, userID int64, content string) (*store.Memory, error) {
	return s.store.CreateMemory(ctx, userID, strings.TrimSpace(content), "")
}

// Relevant returns memories relevant to arbitrary text (e.g. the user's message),
// for prompt injection. Never errors — returns nil on any problem. Query
// sanitization is the store backend's responsibility, so raw text is passed
// through.
func (s *Service) Relevant(ctx context.Context, userID int64, text string, limit int) []store.Memory {
	mem, err := s.store.SearchMemories(ctx, userID, text, limit)
	if err != nil {
		return nil
	}
	return mem
}

// Search runs an explicit recall query (used by the recall tool). Raw text is
// passed straight to the store, which sanitizes it for its query dialect.
func (s *Service) Search(ctx context.Context, userID int64, query string, limit int) ([]store.Memory, error) {
	return s.store.SearchMemories(ctx, userID, query, limit)
}
