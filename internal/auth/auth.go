// Package auth handles sign-in, sessions, CSRF and role checks.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

const (
	keyUserID      = "user_id"
	keyAuthVersion = "auth_version"
	keyCSRF        = "csrf"
	keyFlash       = "flash"
	CSRFField      = "csrf_token"
	CSRFHeader     = "X-CSRF-Token"
)

type Manager struct {
	Sessions   *scs.SessionManager
	q          *gen.Queries
	log        *slog.Logger
	ipLimit    *limiter
	emailLimit *limiter
	trustProxy bool
}

func New(pool *pgxpool.Pool, log *slog.Logger, prod bool) *Manager {
	sm := scs.New()
	sm.Store = pgxstore.NewWithCleanupInterval(pool, 30*time.Minute)
	sm.Lifetime = 12 * time.Hour
	sm.IdleTimeout = 4 * time.Hour
	sm.Cookie.Name = "sny_session"
	if prod {
		// __Host- prefix: browser enforces Secure, Path=/ and no Domain.
		sm.Cookie.Name = "__Host-sny_session"
	}
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = prod
	sm.Cookie.Persist = false
	return &Manager{
		Sessions:   sm,
		q:          gen.New(pool),
		log:        log,
		ipLimit:    newLimiter(20, 15*time.Minute),
		emailLimit: newLimiter(5, 15*time.Minute),
		trustProxy: prod,
	}
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func toUser(id int64, name, email, role string, branchID *int16, branchName *string, active bool, ver int32) *model.User {
	u := &model.User{ID: id, Name: name, Email: email, Role: model.Role(role), BranchID: branchID, Active: active, AuthVersion: ver}
	if branchName != nil {
		u.BranchName = *branchName
	}
	return u
}

// Middleware loads the session, ensures a CSRF token, resolves the signed-in
// user (dropping the session if the user was deactivated or reset), pops the
// flash message for full-page GETs, and verifies CSRF on unsafe methods.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tok := m.Sessions.GetString(ctx, keyCSRF)
		if tok == "" {
			tok = newToken()
			m.Sessions.Put(ctx, keyCSRF, tok)
		}
		ctx = reqctx.WithCSRF(ctx, tok)

		if uid := m.Sessions.GetInt64(ctx, keyUserID); uid != 0 {
			u, err := m.loadUser(ctx, uid)
			switch {
			case err != nil:
				m.log.Error("load session user", "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			case u == nil || !u.Active || u.AuthVersion != int32(m.Sessions.GetInt(ctx, keyAuthVersion)):
				_ = m.Sessions.Destroy(ctx)
			default:
				ctx = reqctx.WithUser(ctx, u)
			}
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			sent := r.Header.Get(CSRFHeader)
			if sent == "" {
				sent = r.PostFormValue(CSRFField)
			}
			if subtle.ConstantTimeCompare([]byte(sent), []byte(tok)) != 1 {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_ = pages.Error(http.StatusForbidden, "Please try again",
					"Your session expired or the form was out of date. Go back, reload the page and submit it again.").
					Render(ctx, w)
				return
			}
		} else if r.Header.Get("HX-Request") == "" {
			if f := m.Sessions.PopString(ctx, keyFlash); f != "" {
				ctx = reqctx.WithFlash(ctx, f)
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	return m.Sessions.LoadAndSave(inner)
}

func (m *Manager) loadUser(ctx context.Context, id int64) (*model.User, error) {
	row, err := m.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toUser(row.ID, row.Name, row.Email, row.Role, row.BranchID, row.BranchName, row.Active, row.AuthVersion), nil
}

// Flash stores a one-line message shown on the next full page.
func (m *Manager) Flash(ctx context.Context, msg string) {
	m.Sessions.Put(ctx, keyFlash, msg)
}

// Redirect sends the browser elsewhere, using HX-Redirect for htmx requests.
func Redirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// RequireLogin redirects anonymous visitors to the sign-in page.
func RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqctx.User(r.Context()) == nil {
			Redirect(w, r, "/login")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole allows only the listed roles. Others get a plain 404 so
// admin-only pages aren't advertised.
func RequireRole(notFound http.Handler, roles ...model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := reqctx.User(r.Context())
			for _, role := range roles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			notFound.ServeHTTP(w, r)
		}))
	}
}
