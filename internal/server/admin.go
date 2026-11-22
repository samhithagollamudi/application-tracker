package server

import (
	"net/http"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

// registerAdmin mounts admin tools (milestone 6). Placeholder for now.
func registerAdmin(app *http.ServeMux, am *auth.Manager, d Deps, cat *catalog.Service, admin func(http.Handler) http.Handler) {
	app.Handle("GET /applications", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		web.OK(w, r, pages.Placeholder("Applications", "applications"))
	})))
}
