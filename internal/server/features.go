package server

import (
	"net/http"

	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/students"
)

func registerFeatures(app *http.ServeMux, am *auth.Manager, d Deps, notFound http.Handler) {
	cat := catalog.New(d.Pool)
	appSvc := applications.New(d.Pool)
	sh := &students.Handlers{Svc: students.New(d.Pool), Cat: cat, Apps: appSvc, Pool: d.Pool, Auth: am}
	ah := &applications.Handlers{Svc: appSvc, Cat: cat, Auth: am}

	login := func(f http.HandlerFunc) http.Handler { return auth.RequireLogin(f) }
	creators := auth.RequireRole(notFound, model.RoleConsultant, model.RoleAgentNetwork)
	admin := auth.RequireRole(notFound, model.RoleAdmin)

	app.Handle("GET /students", login(sh.List))
	app.Handle("GET /students/new", creators(http.HandlerFunc(sh.NewForm)))
	app.Handle("POST /students/new", creators(http.HandlerFunc(sh.Create)))
	app.Handle("GET /students/{id}", login(sh.Detail))
	app.Handle("GET /students/{id}/edit", login(sh.EditForm))
	app.Handle("POST /students/{id}/edit", login(sh.Update))
	app.Handle("POST /students/{id}/move", admin(http.HandlerFunc(sh.Move)))

	app.Handle("GET /students/{id}/applications/new", login(ah.NewForm))
	app.Handle("POST /students/{id}/applications/new", login(ah.Create))
	app.Handle("GET /applications/{id}/edit", login(ah.EditForm))
	app.Handle("POST /applications/{id}/edit", login(ah.Update))
	app.Handle("POST /applications/{id}/status", login(ah.Status))
	app.Handle("POST /applications/{id}/delete", login(ah.Delete))
	app.Handle("GET /catalog/universities/search", login(ah.SearchUniversities))

	registerAdmin(app, am, d, cat, ah, admin)
}
