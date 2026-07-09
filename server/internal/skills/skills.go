// Package skills exposes the user's enabled skills to the agent, so their
// prompts can be injected and their tools gated per user.
package skills

import (
	"context"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Service resolves a user's enabled skills.
type Service struct {
	store store.Store
}

// New creates a skills service.
func New(s store.Store) *Service {
	return &Service{store: s}
}

// Enabled returns the skills enabled for the user (with prompt + key).
func (s *Service) Enabled(ctx context.Context, userID int64) []store.Skill {
	list, err := s.store.ListUserSkills(ctx, userID)
	if err != nil {
		return nil
	}
	var out []store.Skill
	for _, u := range list {
		if u.Enabled {
			out = append(out, u.Skill)
		}
	}
	return out
}
