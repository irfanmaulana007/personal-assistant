package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type skillResp struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Enabled     bool   `json:"enabled"`
	AutoTuned   bool   `json:"auto_tuned"` // the self-tuner has overridden this skill's prompt
	// Scope is "global" for a shared, code-seeded skill or "project" for a fork
	// this project owns and customized.
	Scope string `json:"scope"`
	// Prompt management fields. Populated only for the caller allowed to edit
	// this skill's prompt — a superadmin for a global skill, a project admin for
	// a project fork — since the prompt is internal behaviour a member never
	// needs to see.
	Prompt          string  `json:"prompt,omitempty"`
	DefaultPrompt   string  `json:"default_prompt,omitempty"`
	PromptUpdatedAt *string `json:"prompt_updated_at,omitempty"`
	PromptUpdatedBy string  `json:"prompt_updated_by,omitempty"`
}

const (
	skillScopeGlobal  = "global"
	skillScopeProject = "project"
)

// skillScope reports the scope tag for a skill row.
func skillScope(sk store.Skill) string {
	if sk.IsProjectOwned() {
		return skillScopeProject
	}
	return skillScopeGlobal
}

// handleListSkills returns the active project's skills with effective enabled
// state (folding in the feature-cascade gate). For superadmins each skill
// additionally carries its editable prompt, the code-owned default, and who last
// edited it (the prompt catalog is platform-wide).
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		pid, _ = s.defaultProject(ctx, claims)
	}
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	list, err := s.store.ListProjectSkills(ctx, pid)
	if err != nil {
		s.log.Error("list project skills", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	superadmin := claims.Role == store.GlobalRoleSuperadmin
	role := authctx.ProjectRole(ctx)
	projectAdmin := superadmin || role == store.ProjectRoleAdmin
	out := make([]skillResp, 0, len(list))
	for _, u := range list {
		scope := skillScope(u.Skill)
		resp := skillResp{
			ID: u.ID, Key: u.Key, Name: u.Name, Description: u.Description, Category: u.Category, Enabled: u.Enabled,
			AutoTuned: u.TunedPrompt != "", Scope: scope,
		}
		// A global skill's prompt is superadmin-owned (it applies platform-wide);
		// a fork's prompt is owned by the project's admins. Show prompt fields only
		// to whoever may edit that scope.
		canSeePrompt := (scope == skillScopeGlobal && superadmin) || (scope == skillScopeProject && projectAdmin)
		if canSeePrompt {
			resp.Prompt = u.Prompt
			resp.DefaultPrompt = store.DefaultSkillPrompt(u.Key)
			resp.PromptUpdatedBy = u.PromptUpdatedBy
			if u.PromptUpdatedAt != nil {
				ts := u.PromptUpdatedAt.Format(time.RFC3339)
				resp.PromptUpdatedAt = &ts
			}
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleResetSkillPrompt clears a skill's auto-tuned prompt override, reverting
// it to the shipped default. Admin-only (it affects the shared skill catalog).
func (s *Server) handleResetSkillPrompt(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}
	sk, err := s.store.GetSkill(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if err := s.store.UpdateSkillTunedPrompt(r.Context(), sk.Key, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset skill prompt"})
		return
	}
	s.handleListSkills(w, r)
}

// handleSetSkillPrompt updates a skill's prompt. Scope decides who may edit it
// and what "reset" means:
//   - Global skill: superadmin only. The prompt is platform-wide; reset restores
//     the code-owned default and hands the prompt back to the boot seed.
//   - Project fork: a project admin of the owning project. The prompt applies to
//     that project only; reset restores the code-owned default of its base key
//     but keeps the fork (still project-owned). Removing the fork entirely is a
//     separate DELETE.
//
// Runs behind the `project` middleware, so the active project id and the
// caller's role in it are on the context.
func (s *Server) handleSetSkillPrompt(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
		Reset  bool   `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ctx := r.Context()
	sk, err := s.store.GetSkill(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	superadmin := claims.Role == store.GlobalRoleSuperadmin
	if !sk.IsProjectOwned() {
		// Global skill: superadmin-only, platform-wide edit.
		if !superadmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "superadmin access required to edit a global skill's prompt"})
			return
		}
		if req.Reset {
			// Restore the shipped default and hand the prompt back to the boot seed
			// (empty updatedBy clears the customization stamp).
			if err := s.store.SetSkillPrompt(ctx, id, store.DefaultSkillPrompt(sk.Key), ""); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
				return
			}
			s.handleListSkills(w, r)
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt cannot be empty"})
			return
		}
		if err := s.store.SetSkillPrompt(ctx, id, prompt, claims.Email); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
			return
		}
		s.handleListSkills(w, r)
		return
	}

	// Project fork: only an admin of the owning project (or a superadmin) may edit.
	pid := authctx.ProjectID(ctx)
	role := authctx.ProjectRole(ctx)
	if sk.ProjectID == nil || *sk.ProjectID != pid {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "skill does not belong to the active project"})
		return
	}
	if !superadmin && role != store.ProjectRoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "project admin access required"})
		return
	}
	// For a fork, reset means restore the base skill's default prompt while
	// keeping the fork owned by the project (stamp stays set).
	prompt := strings.TrimSpace(req.Prompt)
	if req.Reset {
		prompt = store.DefaultSkillPrompt(sk.Key)
	}
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt cannot be empty"})
		return
	}
	if err := s.store.SetSkillPrompt(ctx, id, prompt, claims.Email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
		return
	}
	s.recordAudit(ctx, pid, claims, "skill_prompt_edit", sk.Key, nil)
	s.handleListSkills(w, r)
}

// handleCustomizeSkill forks a global skill into the active project so its
// admins can give it a project-specific prompt. Idempotent: if the project
// already has a fork of that skill it just returns the current list. Behind the
// `projectAdmin` middleware.
func (s *Server) handleCustomizeSkill(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}
	sk, err := s.store.GetSkill(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if sk.IsProjectOwned() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill is already project-specific"})
		return
	}

	// Carry the skill's current effective enabled state in this project over to
	// the fork (which will shadow the global skill once it exists), and bail out
	// idempotently if a fork already exists for this key.
	existing, err := s.store.ListProjectSkills(ctx, pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	enabledBefore := sk.DefaultEnabled
	for _, u := range existing {
		if u.Key != sk.Key {
			continue
		}
		if u.IsProjectOwned() {
			// Already customized — nothing to do.
			s.handleListSkills(w, r)
			return
		}
		enabledBefore = u.Enabled
	}

	base := store.Skill{
		Key:            sk.Key,
		Name:           sk.Name,
		Description:    sk.Description,
		Prompt:         sk.EffectivePrompt(), // seed the fork with what the project sees today
		Category:       sk.Category,
		DefaultEnabled: sk.DefaultEnabled,
		SortOrder:      sk.SortOrder,
	}
	fork, err := s.store.CreateProjectSkill(ctx, pid, base, claims.Email)
	if err != nil {
		s.log.Error("customize skill", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to customize skill"})
		return
	}
	if err := s.store.SetProjectSkillEnabled(ctx, pid, fork.ID, enabledBefore); err != nil {
		s.log.Error("carry over skill enable", "error", err)
	}
	s.recordAudit(ctx, pid, claims, "skill_customize", sk.Key, nil)
	s.handleListSkills(w, r)
}

// handleDeleteSkill removes the active project's fork of a skill, reverting the
// project to the shared global skill. Behind the `projectAdmin` middleware; it
// only ever deletes a fork that belongs to the active project, so a global
// skill can never be removed here.
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}
	sk, err := s.store.GetSkill(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil || !sk.IsProjectOwned() || sk.ProjectID == nil || *sk.ProjectID != pid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project skill not found"})
		return
	}
	if err := s.store.DeleteProjectSkill(ctx, pid, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove customization"})
		return
	}
	s.recordAudit(ctx, pid, claims, "skill_reset", sk.Key, nil)
	s.handleListSkills(w, r)
}

// handleSetSkill enables/disables a skill for the active project. Behind
// withProject + requireProjectAdmin, so the caller is a project admin (or
// superadmin) acting on that project.
func (s *Server) handleSetSkill(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sk, err := s.store.GetSkill(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	if err := s.store.SetProjectSkillEnabled(ctx, pid, id, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
		return
	}
	s.recordAudit(ctx, pid, claims, "skill_toggle", sk.Key, map[string]any{"enabled": req.Enabled})
	s.handleListSkills(w, r)
}
