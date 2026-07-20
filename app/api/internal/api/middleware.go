package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

type contextKey string

const claimsKey contextKey = "claims"

// claimsFrom returns the authenticated claims from context, or nil.
func claimsFrom(ctx context.Context) *jwtClaims {
	claims, _ := ctx.Value(claimsKey).(*jwtClaims)
	return claims
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization format"})
			return
		}

		claims, err := s.validateToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = authctx.WithUserID(ctx, claims.UserID())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireSuperadmin wraps a handler so only global superadmins may call it. This
// guards all platform-wide surfaces (settings, users, pricing, logs,
// integrations, whatsapp, routines).
func (s *Server) requireSuperadmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFrom(r.Context())
		if claims == nil || claims.Role != store.GlobalRoleSuperadmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "superadmin access required"})
			return
		}
		next(w, r)
	}
}

// withProject resolves the active project for a data request from the
// X-Project-Id header (falling back to the caller's default project) and stores
// the project id and the caller's role in it on the context, so the store scopes
// domain data to that project. A superadmin may act on any project; any other
// user must be a member. Used for the domain-data routes (chat, reminders,
// bucket list, …).
func (s *Server) withProject(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFrom(r.Context())
		if claims == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		ctx := r.Context()
		pid, _ := strconv.ParseInt(r.Header.Get("X-Project-Id"), 10, 64)
		superadmin := claims.Role == store.GlobalRoleSuperadmin

		role := ""
		if pid > 0 {
			if superadmin {
				p, err := s.store.GetProject(ctx, pid)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
					return
				}
				if p == nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				role = store.GlobalRoleSuperadmin
			} else {
				r2, err := s.store.GetProjectRole(ctx, pid, claims.UserID())
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
					return
				}
				if r2 == "" {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this project"})
					return
				}
				role = r2
			}
		} else {
			// No explicit project: fall back to the caller's default project so the
			// app and the WhatsApp/owner path always have a project to act in.
			pid, role = s.defaultProject(ctx, claims)
			if pid == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active project"})
				return
			}
		}

		ctx = authctx.WithProjectID(ctx, pid)
		ctx = authctx.WithProjectRole(ctx, role)
		next(w, r.WithContext(ctx))
	}
}

// defaultProject picks a fallback active project for a caller who sent no
// X-Project-Id: their first membership, or (for a superadmin with none) the
// first project overall. Returns (0, "") when there is none.
func (s *Server) defaultProject(ctx context.Context, claims *jwtClaims) (int64, string) {
	summaries, err := s.store.ListProjectsForUser(ctx, claims.UserID())
	if err == nil && len(summaries) > 0 {
		return summaries[0].ID, summaries[0].Role
	}
	if claims.Role == store.GlobalRoleSuperadmin {
		projects, err := s.store.ListProjects(ctx)
		if err == nil && len(projects) > 0 {
			return projects[0].ID, store.GlobalRoleSuperadmin
		}
	}
	return 0, ""
}

// requireProjectAdmin wraps a data handler (already behind withProject) so only a
// project admin (or a superadmin) may call it.
func (s *Server) requireProjectAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := authctx.ProjectRole(r.Context())
		if role != store.ProjectRoleAdmin && role != store.GlobalRoleSuperadmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "project admin access required"})
			return
		}
		next(w, r)
	}
}

// projectAccess resolves the {id} path param for a /api/projects/{id}… handler,
// verifies the caller may access that project (membership, or superadmin), and
// returns the project id and the caller's effective role. When needAdmin, it
// additionally requires the project admin role (superadmin always passes). On
// failure it writes the error response and returns ok=false.
func (s *Server) projectAccess(w http.ResponseWriter, r *http.Request, needAdmin bool) (int64, string, bool) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return 0, "", false
	}
	pid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || pid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project id"})
		return 0, "", false
	}
	role := ""
	if claims.Role == store.GlobalRoleSuperadmin {
		role = store.GlobalRoleSuperadmin
	} else {
		r2, err := s.store.GetProjectRole(r.Context(), pid, claims.UserID())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return 0, "", false
		}
		if r2 == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this project"})
			return 0, "", false
		}
		role = r2
	}
	if needAdmin && role != store.ProjectRoleAdmin && role != store.GlobalRoleSuperadmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "project admin access required"})
		return 0, "", false
	}
	return pid, role, true
}

// isSuperadmin reports whether the request's caller holds the global superadmin
// role.
func (s *Server) isSuperadmin(r *http.Request) bool {
	claims := claimsFrom(r.Context())
	return claims != nil && claims.Role == store.GlobalRoleSuperadmin
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Debug("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", time.Since(start),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
