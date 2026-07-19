package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// --- DTOs ---

type projectResp struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	OwnerUserID int64  `json:"owner_user_id"`
	Role        string `json:"role"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
}

type memberResp struct {
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type projectFeatureResp struct {
	ID          int64    `json:"id"`
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	SkillKeys   []string `json:"skill_keys"`
}

type auditResp struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	ActorEmail string `json:"actor_email"`
	Metadata   string `json:"metadata"`
	CreatedAt  string `json:"created_at"`
}

// --- Helpers ---

// provisionPersonalProject creates a personal project owned by the user and adds
// them as its admin. Called when a user is created (setup or admin-created) so no
// account is ever left with zero projects.
func (s *Server) provisionPersonalProject(ctx context.Context, u *store.User) error {
	name := strings.TrimSpace(u.Name)
	if name == "" {
		name = u.Email
	}
	p, err := s.store.CreateProject(ctx, name+" — Personal", u.ID)
	if err != nil {
		return err
	}
	return s.store.AddProjectMember(ctx, p.ID, u.ID, store.ProjectRoleAdmin)
}

// recordAudit appends a project-level action to the audit log, best-effort (a
// logging failure never blocks the action).
func (s *Server) recordAudit(ctx context.Context, projectID int64, claims *jwtClaims, action, target string, metadata map[string]any) {
	meta := ""
	if len(metadata) > 0 {
		if b, err := json.Marshal(metadata); err == nil {
			meta = string(b)
		}
	}
	email := ""
	var actor int64
	if claims != nil {
		email = claims.Email
		actor = claims.UserID()
	}
	if err := s.store.RecordAudit(ctx, &store.AuditEvent{
		ProjectID:   projectID,
		ActorUserID: actor,
		ActorEmail:  email,
		Action:      action,
		Target:      target,
		Metadata:    meta,
	}); err != nil {
		s.log.Error("record audit", "error", err, "action", action)
	}
}

// --- Projects ---

// handleListProjects lists the projects visible to the caller: every project for
// a superadmin, otherwise the ones they belong to.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	out := []projectResp{}
	if claims.Role == store.GlobalRoleSuperadmin {
		projects, err := s.store.ListProjects(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load projects"})
			return
		}
		for _, p := range projects {
			members, _ := s.store.ListProjectMembers(ctx, p.ID)
			out = append(out, projectResp{
				ID: p.ID, Name: p.Name, OwnerUserID: p.OwnerUserID,
				Role: store.GlobalRoleSuperadmin, MemberCount: len(members),
				CreatedAt: p.CreatedAt.Format(time.RFC3339),
			})
		}
	} else {
		summaries, err := s.store.ListProjectsForUser(ctx, claims.UserID())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load projects"})
			return
		}
		for _, p := range summaries {
			out = append(out, projectResp{
				ID: p.ID, Name: p.Name, OwnerUserID: p.OwnerUserID, Role: p.Role,
				MemberCount: p.MemberCount, CreatedAt: p.CreatedAt.Format(time.RFC3339),
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateProject creates a project and names its initial admin. Superadmin
// only. Body: {name, admin_email?} — admin_email defaults to the caller.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		Name       string `json:"name"`
		AdminEmail string `json:"admin_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project name is required"})
		return
	}
	ctx := r.Context()

	// Resolve the initial admin: the named user, or the caller.
	adminID := claims.UserID()
	if email := strings.ToLower(strings.TrimSpace(req.AdminEmail)); email != "" {
		u, err := s.store.GetUserByEmail(ctx, email)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if u == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no user with that email"})
			return
		}
		adminID = u.ID
	}

	p, err := s.store.CreateProject(ctx, name, adminID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create project"})
		return
	}
	if err := s.store.AddProjectMember(ctx, p.ID, adminID, store.ProjectRoleAdmin); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add project admin"})
		return
	}
	s.recordAudit(ctx, p.ID, claims, "project_create", name, map[string]any{"admin_user_id": adminID})
	members, _ := s.store.ListProjectMembers(ctx, p.ID)
	writeJSON(w, http.StatusOK, projectResp{
		ID: p.ID, Name: p.Name, OwnerUserID: p.OwnerUserID, Role: store.GlobalRoleSuperadmin,
		MemberCount: len(members), CreatedAt: p.CreatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	pid, role, ok := s.projectAccess(w, r, false)
	if !ok {
		return
	}
	p, err := s.store.GetProject(r.Context(), pid)
	if err != nil || p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	members, _ := s.store.ListProjectMembers(r.Context(), pid)
	writeJSON(w, http.StatusOK, projectResp{
		ID: p.ID, Name: p.Name, OwnerUserID: p.OwnerUserID, Role: role,
		MemberCount: len(members), CreatedAt: p.CreatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project name is required"})
		return
	}
	if err := s.store.UpdateProjectName(r.Context(), pid, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to rename project"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "project_rename", name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteProject(r.Context(), pid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete project"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Members ---

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, false)
	if !ok {
		return
	}
	members, err := s.store.ListProjectMembers(r.Context(), pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}
	out := make([]memberResp, 0, len(members))
	for _, m := range members {
		out = append(out, memberResp{
			UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAddMember adds an existing user (by email) to the project. A project
// admin may only add members; appointing an admin requires superadmin.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = store.ProjectRoleMember
	}
	if role != store.ProjectRoleAdmin && role != store.ProjectRoleMember {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or member"})
		return
	}
	if role == store.ProjectRoleAdmin && !s.isSuperadmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a superadmin can appoint a project admin"})
		return
	}
	u, err := s.store.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if u == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no user with that email"})
		return
	}
	if err := s.store.AddProjectMember(r.Context(), pid, u.ID, role); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add member"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "member_invite", u.Email, map[string]any{"role": role})
	s.handleListMembers(w, r)
}

// handleCreateMember creates a brand-new user account and adds them to the
// project in one step. This lets a project admin onboard someone who has no
// account yet (handleAddMember only attaches an existing user). The created
// user always gets the global "member" role — a project admin cannot mint a
// global superadmin — while appointing them a *project* admin stays superadmin
// only, matching handleAddMember.
func (s *Server) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = store.ProjectRoleMember
	}
	if role != store.ProjectRoleAdmin && role != store.ProjectRoleMember {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or member"})
		return
	}
	if role == store.ProjectRoleAdmin && !s.isSuperadmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a superadmin can appoint a project admin"})
		return
	}
	if msg := validateCredentials(credentials{Email: req.Email, Password: req.Password}); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	existing, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a user with that email already exists"})
		return
	}
	user, err := s.createUser(r, email, req.Password, store.GlobalRoleMember)
	if err != nil {
		s.log.Error("create member user", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
		return
	}
	// Unlike handleCreateUser, we do NOT provision a personal project here: the
	// user is created from within a project and is added to it right below, so
	// they're never stranded with zero projects. Creating an extra personal
	// project would be a surprise side effect of onboarding someone to a project.
	if err := s.store.AddProjectMember(r.Context(), pid, user.ID, role); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add member"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "member_create", user.Email, map[string]any{"role": role})
	s.handleListMembers(w, r)
}

// handleUpdateMember changes a member's project role. Promoting to or demoting
// from admin requires superadmin; the last admin cannot be demoted.
func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	targetID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	newRole := strings.ToLower(strings.TrimSpace(req.Role))
	if newRole != store.ProjectRoleAdmin && newRole != store.ProjectRoleMember {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or member"})
		return
	}
	current, _ := s.store.GetProjectRole(r.Context(), pid, targetID)
	if current == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user is not a member"})
		return
	}
	// Any change that touches the admin role is superadmin-only.
	if (newRole == store.ProjectRoleAdmin || current == store.ProjectRoleAdmin) && !s.isSuperadmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a superadmin can change the admin role"})
		return
	}
	// Don't demote the last admin.
	if current == store.ProjectRoleAdmin && newRole != store.ProjectRoleAdmin {
		if n, _ := s.store.CountProjectAdmins(r.Context(), pid); n <= 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a project must keep at least one admin"})
			return
		}
	}
	if err := s.store.UpdateProjectMemberRole(r.Context(), pid, targetID, newRole); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update member"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "member_role", strconv.FormatInt(targetID, 10), map[string]any{"role": newRole})
	s.handleListMembers(w, r)
}

// handleRemoveMember removes a member. A project admin cannot remove another
// admin (superadmin only); the last admin cannot be removed.
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	targetID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	current, _ := s.store.GetProjectRole(r.Context(), pid, targetID)
	if current == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if current == store.ProjectRoleAdmin {
		if !s.isSuperadmin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a superadmin can remove a project admin"})
			return
		}
		if n, _ := s.store.CountProjectAdmins(r.Context(), pid); n <= 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a project must keep at least one admin"})
			return
		}
	}
	if err := s.store.RemoveProjectMember(r.Context(), pid, targetID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove member"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "member_remove", strconv.FormatInt(targetID, 10), nil)
	s.handleListMembers(w, r)
}

// --- Features (active project, header-scoped like /api/skills) ---

func (s *Server) handleListFeatures(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		pid, _ = s.defaultProject(ctx, claims)
	}
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	feats, err := s.store.ListProjectFeatures(ctx, pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load features"})
		return
	}
	out := make([]projectFeatureResp, 0, len(feats))
	for _, f := range feats {
		out = append(out, projectFeatureResp{
			ID: f.ID, Key: f.Key, Name: f.Name, Description: f.Description,
			Enabled: f.Enabled, SkillKeys: f.SkillKeys,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetFeature enables/disables a feature for the active project (project
// admin). Disabling cascades: the store's effective-skill query treats every
// skill under a disabled feature as off.
func (s *Server) handleSetFeature(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	ctx := r.Context()
	pid := authctx.ProjectID(ctx)
	if pid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid feature id"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	f, err := s.store.GetFeature(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if f == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "feature not found"})
		return
	}
	if err := s.store.SetProjectFeatureEnabled(ctx, pid, id, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update feature"})
		return
	}
	s.recordAudit(ctx, pid, claims, "feature_toggle", f.Key, map[string]any{"enabled": req.Enabled})
	s.handleListFeatures(w, r)
}

// --- Per-project skills (path-scoped: manage a specific project's skills) ---

type projectSkillResp struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) handleProjectSkills(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, false)
	if !ok {
		return
	}
	list, err := s.store.ListProjectSkills(r.Context(), pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	out := make([]projectSkillResp, 0, len(list))
	for _, u := range list {
		out = append(out, projectSkillResp{
			ID: u.ID, Key: u.Key, Name: u.Name, Description: u.Description,
			Category: u.Category, Enabled: u.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetProjectSkill enables/disables a skill for a specific project (by
// path). Requires project admin (or superadmin) on that project.
func (s *Server) handleSetProjectSkill(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	skillID, err := strconv.ParseInt(r.PathValue("skillId"), 10, 64)
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
	sk, err := s.store.GetSkill(r.Context(), skillID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if sk == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if err := s.store.SetProjectSkillEnabled(r.Context(), pid, skillID, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "skill_toggle", sk.Key, map[string]any{"enabled": req.Enabled})
	s.handleProjectSkills(w, r)
}

// --- Per-project features (path-scoped) ---

func (s *Server) handleProjectFeatures(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, false)
	if !ok {
		return
	}
	feats, err := s.store.ListProjectFeatures(r.Context(), pid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load features"})
		return
	}
	out := make([]projectFeatureResp, 0, len(feats))
	for _, f := range feats {
		out = append(out, projectFeatureResp{
			ID: f.ID, Key: f.Key, Name: f.Name, Description: f.Description,
			Enabled: f.Enabled, SkillKeys: f.SkillKeys,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetProjectFeature enables/disables a feature for a specific project (by
// path); disabling cascades to its skills. Requires project admin / superadmin.
func (s *Server) handleSetProjectFeature(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	featureID, err := strconv.ParseInt(r.PathValue("featureId"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid feature id"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	f, err := s.store.GetFeature(r.Context(), featureID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if f == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "feature not found"})
		return
	}
	if err := s.store.SetProjectFeatureEnabled(r.Context(), pid, featureID, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update feature"})
		return
	}
	s.recordAudit(r.Context(), pid, claimsFrom(r.Context()), "feature_toggle", f.Key, map[string]any{"enabled": req.Enabled})
	s.handleProjectFeatures(w, r)
}

// --- Audit ---

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	pid, _, ok := s.projectAccess(w, r, true)
	if !ok {
		return
	}
	events, err := s.store.ListAuditEvents(r.Context(), pid, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load audit log"})
		return
	}
	out := make([]auditResp, 0, len(events))
	for _, e := range events {
		out = append(out, auditResp{
			ID: e.ID, Action: e.Action, Target: e.Target, ActorEmail: e.ActorEmail,
			Metadata: e.Metadata, CreatedAt: e.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
