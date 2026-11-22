// Package reqctx carries per-request values (signed-in user, CSRF token,
// flash message) so templates can read them without importing auth.
package reqctx

import (
	"context"

	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

type key int

const (
	userKey key = iota
	csrfKey
	flashKey
)

func WithUser(ctx context.Context, u *model.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// User returns the signed-in user, or nil.
func User(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

func WithCSRF(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfKey, token)
}

func CSRF(ctx context.Context) string {
	s, _ := ctx.Value(csrfKey).(string)
	return s
}

func WithFlash(ctx context.Context, msg string) context.Context {
	return context.WithValue(ctx, flashKey, msg)
}

func Flash(ctx context.Context) string {
	s, _ := ctx.Value(flashKey).(string)
	return s
}
