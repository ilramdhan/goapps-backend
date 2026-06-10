package costproductrequest

import "context"

// ctxKey is a local unexported type to avoid collisions when reading context values.
// The string values MUST match the delivery-layer AuthUserIDKey / AuthUsernameKey constants.
type ctxKey string

const (
	ctxKeyUserID   ctxKey = "auth_user_id"
	ctxKeyUsername ctxKey = "auth_username"
)

// actorIDFromCtx extracts the authenticated user's UUID string from context.
// Key value "auth_user_id" matches AuthUserIDKey in the delivery layer.
func actorIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyUserID).(string)
	return id
}

// actorNameFromCtx extracts the authenticated user's username from context.
// Key value "auth_username" matches AuthUsernameKey in the delivery layer.
// Falls back to actor ID when username is absent.
func actorNameFromCtx(ctx context.Context) string {
	name, _ := ctx.Value(ctxKeyUsername).(string)
	if name == "" {
		return actorIDFromCtx(ctx)
	}
	return name
}
