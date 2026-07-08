// Package authctx carries the authenticated user id through context so the
// capability handlers and store can scope data per user without importing the
// api package.
package authctx

import "context"

type ctxKey struct{}

var userIDKey ctxKey

// WithUserID returns a context carrying the given user id.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserID returns the user id carried by ctx, or 0 if none.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}
