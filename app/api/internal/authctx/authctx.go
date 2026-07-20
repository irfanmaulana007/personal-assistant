// Package authctx carries the authenticated user id — and, for multi-project
// RBAC, the active project id and the caller's role in that project — through
// context so the capability handlers and store can scope data per project
// without importing the api package.
package authctx

import "context"

// ctxKey is a private, typed context key. Distinct constant values keep the
// keys from colliding (an empty struct would make every key equal).
type ctxKey int

const (
	userIDKey ctxKey = iota
	projectIDKey
	projectRoleKey
)

// WithUserID returns a context carrying the given user id.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserID returns the user id carried by ctx, or 0 if none.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}

// WithProjectID returns a context carrying the active project id. Requests that
// carry no project (background jobs like the reminder scheduler) leave it unset,
// and stores treat a zero project id as "not project-scoped".
func WithProjectID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, projectIDKey, id)
}

// ProjectID returns the active project id carried by ctx, or 0 if none.
func ProjectID(ctx context.Context) int64 {
	id, _ := ctx.Value(projectIDKey).(int64)
	return id
}

// WithProjectRole returns a context carrying the caller's role in the active
// project ("admin" | "member"), or "superadmin" when the caller is a global
// superadmin acting on the project.
func WithProjectRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, projectRoleKey, role)
}

// ProjectRole returns the caller's role in the active project, or "" if none.
func ProjectRole(ctx context.Context) string {
	role, _ := ctx.Value(projectRoleKey).(string)
	return role
}
