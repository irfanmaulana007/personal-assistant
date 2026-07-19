package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// validRole reports whether role is an assignable global role. Global roles are
// superadmin (unrestricted) and member; project-scoped admin/member live in
// project_members, not here.
func validRole(role string) bool {
	return role == store.GlobalRoleSuperadmin || role == store.GlobalRoleMember
}

// handleListUsers returns all users (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.log.Error("list users", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load users"})
		return
	}
	out := make([]userResp, 0, len(users))
	for i := range users {
		out = append(out, toUserResp(&users[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// handleCreateUser creates a new user (admin only).
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Role == "" {
		req.Role = store.GlobalRoleMember
	}
	if !validRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be superadmin or member"})
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

	user, err := s.createUser(r, email, req.Password, req.Role)
	if err != nil {
		s.log.Error("create user", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
		return
	}
	// Give every new member their own personal project (admin of it) so they are
	// never stranded with zero projects. Superadmins manage every project and are
	// intentionally left unattached to any single one, so they get no personal one.
	if user.Role != store.GlobalRoleSuperadmin {
		if err := s.provisionPersonalProject(r.Context(), user); err != nil {
			s.log.Error("provision personal project", "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, toUserResp(user))
}

type updateUserRequest struct {
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

// handleUpdateUser updates a user's role and/or password (admin only).
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	target, err := s.store.GetUserByID(r.Context(), id)
	if err != nil || target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	if req.Role != nil {
		if !validRole(*req.Role) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be superadmin or member"})
			return
		}
		// Prevent removing the last superadmin.
		if target.Role == store.GlobalRoleSuperadmin && *req.Role != store.GlobalRoleSuperadmin {
			if ok, err := s.isLastSuperadmin(r, target.ID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			} else if ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot demote the last superadmin"})
				return
			}
		}
		if err := s.store.UpdateUserRole(r.Context(), id, *req.Role); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update role"})
			return
		}
		// Promoting a member to superadmin detaches them from every project: a
		// superadmin manages all projects globally and is not a member of any.
		if *req.Role == store.GlobalRoleSuperadmin && target.Role != store.GlobalRoleSuperadmin {
			if projs, err := s.store.ListProjectsForUser(r.Context(), id); err != nil {
				s.log.Error("list projects for detach", "error", err, "user", id)
			} else {
				for _, p := range projs {
					if err := s.store.RemoveProjectMember(r.Context(), p.ID, id); err != nil {
						s.log.Error("detach superadmin from project", "error", err, "user", id, "project", p.ID)
					}
				}
			}
		}
	}

	if req.Password != nil {
		if len(*req.Password) < minPasswordLen {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password too short"})
			return
		}
		hash, err := hashPassword(*req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if err := s.store.UpdateUserPassword(r.Context(), id, hash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
			return
		}
	}

	updated, _ := s.store.GetUserByID(r.Context(), id)
	writeJSON(w, http.StatusOK, toUserResp(updated))
}

// handleDeleteUser deletes a user (admin only). Cannot delete self or the last admin.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	claims := claimsFrom(r.Context())
	if claims != nil && claims.UserID() == id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "you cannot delete your own account"})
		return
	}

	target, err := s.store.GetUserByID(r.Context(), id)
	if err != nil || target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if target.Role == store.GlobalRoleSuperadmin {
		if ok, err := s.isLastSuperadmin(r, target.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		} else if ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete the last superadmin"})
			return
		}
	}

	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete user"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// isLastSuperadmin reports whether the given superadmin user is the only one.
func (s *Server) isLastSuperadmin(r *http.Request, id int64) (bool, error) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		return false, err
	}
	admins := 0
	for _, u := range users {
		if u.Role == store.GlobalRoleSuperadmin {
			admins++
		}
	}
	return admins <= 1, nil
}
