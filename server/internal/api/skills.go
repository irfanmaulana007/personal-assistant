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
	IsCore      bool   `json:"is_core"`    // a core skill (auto-available to every project)
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
			IsCore: u.IsCore, AutoTuned: u.TunedPrompt != "", Scope: scope,
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

// adminProjectRef is a project a skill maps to, in the superadmin catalog.
type adminProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// adminSkillResp is one skill in the platform-wide (superadmin) catalog: the
// skill plus its storage scope, its core flag, the projects that effectively
// enable it, and the derived classification the /skills tabs bucket it under.
type adminSkillResp struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Scope       string `json:"scope"` // storage scope: "global" | "project"
	IsCore      bool   `json:"is_core"`
	// Classification is the taxonomy the UI tabs on, derived from is_core and the
	// project mapping: "core" | "global" | "project".
	Classification  string            `json:"classification"`
	AutoTuned       bool              `json:"auto_tuned"`
	Projects        []adminProjectRef `json:"projects"`
	Prompt          string            `json:"prompt,omitempty"`
	DefaultPrompt   string            `json:"default_prompt,omitempty"`
	PromptUpdatedAt *string           `json:"prompt_updated_at,omitempty"`
	PromptUpdatedBy string            `json:"prompt_updated_by,omitempty"`
}

// classifySkill derives a skill's catalog classification from its project
// mapping. Core (the superadmin flag) always wins. A fork is owned by one
// project, so it is project-specific. A global skill is project-specific only
// when exactly one of several projects uses it — when there is a single project
// in the system, "used by one" is the same as "used by all", so it stays
// global. Everything else (used by all/most/none) is a global skill.
func classifySkill(isCore, projectOwned bool, projectCount, totalProjects int) string {
	switch {
	case isCore:
		return "core"
	case projectOwned:
		return "project"
	case totalProjects > 1 && projectCount == 1:
		return "project"
	default:
		return "global"
	}
}

// handleAdminListSkills returns the platform-wide skills catalog for the
// superadmin /skills page: every global skill and every project fork, each with
// its project mapping and derived classification. Superadmin-only (behind the
// `superadmin` middleware), so the editable prompt fields are always included.
func (s *Server) handleAdminListSkills(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListSkillsWithProjectMapping(ctx)
	if err != nil {
		s.log.Error("list skills with mapping", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	// Total project count disambiguates "used by one" from "used by all" when
	// there is only a single project (see classifySkill).
	allProjects, err := s.store.ListProjects(ctx)
	if err != nil {
		s.log.Error("list projects for skill mapping", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	totalProjects := len(allProjects)
	out := make([]adminSkillResp, 0, len(list))
	for _, sm := range list {
		projects := make([]adminProjectRef, 0, len(sm.Projects))
		for _, p := range sm.Projects {
			projects = append(projects, adminProjectRef{ID: p.ID, Name: p.Name, Slug: p.Slug})
		}
		resp := adminSkillResp{
			ID: sm.ID, Key: sm.Key, Name: sm.Name, Description: sm.Description, Category: sm.Category,
			Scope:           skillScope(sm.Skill),
			IsCore:          sm.IsCore,
			Classification:  classifySkill(sm.IsCore, sm.IsProjectOwned(), len(sm.Projects), totalProjects),
			AutoTuned:       sm.TunedPrompt != "",
			Projects:        projects,
			Prompt:          sm.Prompt,
			DefaultPrompt:   store.DefaultSkillPrompt(sm.Key),
			PromptUpdatedBy: sm.PromptUpdatedBy,
		}
		if sm.PromptUpdatedAt != nil {
			ts := sm.PromptUpdatedAt.Format(time.RFC3339)
			resp.PromptUpdatedAt = &ts
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetSkillCore marks or unmarks a global skill as core. Superadmin-only
// (behind the `superadmin` middleware). Project forks cannot be core; the store
// scopes the update to global skills, so a fork id returns 404.
func (s *Server) handleSetSkillCore(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill id"})
		return
	}
	var req struct {
		IsCore bool `json:"is_core"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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
	if sk.IsProjectOwned() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only global skills can be core"})
		return
	}
	if err := s.store.SetSkillCore(r.Context(), id, req.IsCore); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
		return
	}
	s.handleAdminListSkills(w, r)
}

// handleAdminSetSkillPrompt edits (or, with reset, restores) a global skill's
// prompt from the platform-wide /skills page. Superadmin-only and project-
// independent — a fork's prompt is edited from its project instead, so a fork id
// is rejected. Returns the refreshed admin catalog.
func (s *Server) handleAdminSetSkillPrompt(w http.ResponseWriter, r *http.Request) {
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
	sk, err := s.store.GetSkill(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if sk.IsProjectOwned() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "edit a project fork's prompt from its project"})
		return
	}
	if req.Reset {
		// Restore the shipped default and hand the prompt back to the boot seed.
		if err := s.store.SetSkillPrompt(r.Context(), id, store.DefaultSkillPrompt(sk.Key), ""); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
			return
		}
		s.handleAdminListSkills(w, r)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt cannot be empty"})
		return
	}
	if err := s.store.SetSkillPrompt(r.Context(), id, prompt, claims.Email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill prompt"})
		return
	}
	s.handleAdminListSkills(w, r)
}

// handleAdminRevertTuned clears a global skill's auto-tuned prompt override from
// the platform-wide /skills page, reverting it to the shipped default.
// Superadmin-only; returns the refreshed admin catalog.
func (s *Server) handleAdminRevertTuned(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revert skill prompt"})
		return
	}
	s.handleAdminListSkills(w, r)
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
