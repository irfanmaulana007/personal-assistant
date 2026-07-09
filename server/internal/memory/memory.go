// Package memory is the agent's long-term memory: durable facts persisted per
// user and retrieved via SQLite FTS5.
package memory

import (
	"context"
	"strings"
	"unicode"

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
// for prompt injection. Never errors — returns nil on any problem.
func (s *Service) Relevant(ctx context.Context, userID int64, text string, limit int) []store.Memory {
	q := sanitizeFTS(text)
	if q == "" {
		return nil
	}
	mem, err := s.store.SearchMemories(ctx, userID, q, limit)
	if err != nil {
		return nil
	}
	return mem
}

// Search runs an explicit recall query (used by the recall tool).
func (s *Service) Search(ctx context.Context, userID int64, query string, limit int) ([]store.Memory, error) {
	q := sanitizeFTS(query)
	if q == "" {
		return nil, nil
	}
	return s.store.SearchMemories(ctx, userID, q, limit)
}

// sanitizeFTS turns arbitrary text into a safe FTS5 query: lowercase alphanumeric
// tokens (len >= 3), deduped, quoted, and OR-joined — so raw user input can't
// break FTS5 syntax and recall stays broad.
func sanitizeFTS(text string) string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	seen := make(map[string]bool)
	terms := make([]string, 0, 12)
	for _, f := range fields {
		if len([]rune(f)) < 3 || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, `"`+f+`"`)
		if len(terms) >= 12 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}
