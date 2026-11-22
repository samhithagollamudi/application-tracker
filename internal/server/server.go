// Package server wires every feature's handlers into one http.Handler.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/config"
	"github.com/ChandraShekar2001/application-tracker/internal/httpx"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/static"
)

type Deps struct {
	Cfg  config.Config
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// New builds the full application handler.
func New(d Deps) http.Handler {
	am := auth.New(d.Pool, d.Log, d.Cfg.IsProd())
	notFound := http.HandlerFunc(web.NotFound)

	app := http.NewServeMux()
	login := auth.RequireLogin
	adminOnly := auth.RequireRole(notFound, model.RoleAdmin)
	_ = adminOnly

	app.HandleFunc("GET /login", am.LoginPage)
	app.HandleFunc("POST /login", am.Login)
	app.HandleFunc("POST /logout", am.Logout)

	app.Handle("GET /{$}", login(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqctx.User(r.Context()).IsAdmin() {
			http.Redirect(w, r, "/applications", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/students", http.StatusSeeOther)
	})))

	registerFeatures(app, am, d, notFound)

	app.Handle("/", notFound)

	root := http.NewServeMux()
	root.Handle("GET /static/", http.StripPrefix("/static/", static.Handler()))
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(c); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/", am.Middleware(app))

	return httpx.Chain(root,
		httpx.WithRequestID,
		httpx.Logger(d.Log),
		httpx.Recover(d.Log),
		httpx.SecurityHeaders(d.Cfg.IsProd()),
	)
}
