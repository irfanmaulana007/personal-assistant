// Package skills exposes the user's enabled skills to the agent, so their
// prompts can be injected and their tools gated per user.
package skills

import (
	"context"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Service resolves the enabled skills for the active project.
type Service struct {
	store store.Store
}

// New creates a skills service.
func New(s store.Store) *Service {
	return &Service{store: s}
}

// Enabled returns the skills enabled for the active project (with prompt + key),
// folding in the feature-cascade gate. When no project is active (project id 0),
// it falls back to the caller's per-user skills so non-project contexts still
// work.
func (s *Service) Enabled(ctx context.Context, userID int64) []store.Skill {
	// A superadmin scope (e.g. the owner's personal WhatsApp channel) can use every
	// skill, regardless of any project's per-skill/feature config.
	if authctx.ProjectRole(ctx) == store.GlobalRoleSuperadmin {
		if all, err := s.store.ListSkills(ctx); err == nil {
			return all
		}
		return nil
	}
	var (
		list []store.UserSkill
		err  error
	)
	if pid := authctx.ProjectID(ctx); pid > 0 {
		list, err = s.store.ListProjectSkills(ctx, pid)
	} else {
		list, err = s.store.ListUserSkills(ctx, userID)
	}
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
