package server

import (
	"net/http"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

// registerFeatures mounts the feature packages. Placeholders until milestone 3.
func registerFeatures(app *http.ServeMux, am *auth.Manager, d Deps, notFound http.Handler) {
	placeholder := func(title, active string) http.Handler {
		return auth.RequireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			web.OK(w, r, pages.Placeholder(title, active))
		}))
	}
	app.Handle("GET /students", placeholder("Students", "students"))
	app.Handle("GET /applications", placeholder("Applications", "applications"))
}
