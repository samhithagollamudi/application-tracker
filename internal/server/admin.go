package server

import (
	"net/http"

	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/export"
	"github.com/ChandraShekar2001/application-tracker/internal/users"
)

// registerAdmin mounts admin-only tools. Everything here is behind RequireRole(admin).
func registerAdmin(app *http.ServeMux, am *auth.Manager, d Deps, cat *catalog.Service, ah *applications.Handlers, admin func(http.Handler) http.Handler) {
	a := func(f http.HandlerFunc) http.Handler { return admin(f) }

	app.Handle("GET /applications", a(ah.AdminList))
	app.Handle("GET /applications/export.csv", a((&export.Handler{Pool: d.Pool, Log: d.Log}).CSV))

	ch := &catalog.AdminHandlers{Svc: cat, Auth: am}
	app.Handle("GET /admin/catalog", a(ch.Page))
	app.Handle("POST /admin/catalog/{kind}", a(ch.Add))
	app.Handle("POST /admin/catalog/{kind}/{id}/delete", a(ch.Delete))

	uh := &users.Handlers{Pool: d.Pool, Auth: am}
	app.Handle("GET /admin/users", a(uh.Page))
	app.Handle("POST /admin/users", a(uh.Create))
	app.Handle("POST /admin/users/{id}/reset", a(uh.Reset))
	app.Handle("POST /admin/users/{id}/toggle", a(uh.Toggle))
}
