package applications

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

type Handlers struct {
	Svc               *Service
	Cat               *catalog.Service
	Auth              *auth.Manager
	ExportDirectLimit int // shown on the export button: larger exports run in the background
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *Handlers) render(w http.ResponseWriter, r *http.Request, status int, vm pages.AppFormVM) {
	web.Render(w, r, status, pages.AppForm(vm))
}

func (h *Handlers) NewForm(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	name, err := h.Svc.StudentName(r.Context(), access.ScopeOf(reqctx.User(r.Context())), sid)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm := h.Svc.FormVM(model.ApplicationFields{}, nil, "")
	h.fill(&vm, "Add application", fmt.Sprintf("/students/%d/applications/new", sid), sid, name, "Add application")
	h.render(w, r, http.StatusOK, vm)
}

func (h *Handlers) fill(vm *pages.AppFormVM, title, action string, sid int64, name, submit string) {
	vm.Title, vm.Action, vm.Submit = title, action, submit
	vm.CancelHref = fmt.Sprintf("/students/%d", sid)
	vm.Student = name + " · " + model.StudentCode(sid)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sid, ok := pathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	u := reqctx.User(ctx)
	name, err := h.Svc.StudentName(ctx, access.ScopeOf(u), sid)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	f := ParseFields(r)
	in, errs, err := h.Svc.Resolve(ctx, &f)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if errs != nil {
		vm := h.Svc.FormVM(f, errs, in.Country)
		h.fill(&vm, "Add application", fmt.Sprintf("/students/%d/applications/new", sid), sid, name, "Add application")
		h.render(w, r, http.StatusUnprocessableEntity, vm)
		return
	}
	if _, err := h.Svc.Add(ctx, u, sid, in); errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	} else if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(ctx, "Application saved")
	http.Redirect(w, r, fmt.Sprintf("/students/%d", sid), http.StatusSeeOther)
}

func (h *Handlers) load(w http.ResponseWriter, r *http.Request) (*model.User, *model.Application, string, bool) {
	id, ok := pathID(r)
	if !ok {
		web.NotFound(w, r)
		return nil, nil, "", false
	}
	u := reqctx.User(r.Context())
	scope := access.ScopeOf(u)
	a, err := h.Svc.Get(r.Context(), scope, id)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return nil, nil, "", false
	}
	if err != nil {
		web.ServerError(w, r, err)
		return nil, nil, "", false
	}
	name, err := h.Svc.StudentName(r.Context(), scope, a.StudentID)
	if err != nil {
		web.ServerError(w, r, err)
		return nil, nil, "", false
	}
	return u, a, name, true
}

func (h *Handlers) EditForm(w http.ResponseWriter, r *http.Request) {
	_, a, name, ok := h.load(w, r)
	if !ok {
		return
	}
	country := ""
	if !a.Custom {
		country = a.Country
	}
	vm := h.Svc.FormVM(a.ApplicationFields, nil, country)
	h.fill(&vm, "Edit application", fmt.Sprintf("/applications/%d/edit", a.ID), a.StudentID, name, "Save changes")
	h.render(w, r, http.StatusOK, vm)
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	u, a, name, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	f := ParseFields(r)
	in, errs, err := h.Svc.Resolve(ctx, &f)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if errs != nil {
		vm := h.Svc.FormVM(f, errs, in.Country)
		h.fill(&vm, "Edit application", fmt.Sprintf("/applications/%d/edit", a.ID), a.StudentID, name, "Save changes")
		h.render(w, r, http.StatusUnprocessableEntity, vm)
		return
	}
	sid, err := h.Svc.Update(ctx, u, a.ID, in)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(ctx, "Application saved")
	http.Redirect(w, r, fmt.Sprintf("/students/%d", sid), http.StatusSeeOther)
}

// Status is the inline htmx status change; it returns the refreshed card.
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	status := r.PostFormValue("status")
	if status != "" && !model.ValidOption(model.Statuses, status) {
		web.Error(w, r, http.StatusBadRequest)
		return
	}
	a, err := h.Svc.SetStatus(r.Context(), reqctx.User(r.Context()), id, status)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "" {
		h.Auth.Flash(r.Context(), "Status updated")
		http.Redirect(w, r, fmt.Sprintf("/students/%d", a.StudentID), http.StatusSeeOther)
		return
	}
	web.OK(w, r, pages.AppCard(*a))
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	sid, err := h.Svc.Delete(r.Context(), reqctx.User(r.Context()), id)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(r.Context(), "Application deleted")
	auth.Redirect(w, r, fmt.Sprintf("/students/%d", sid))
}

// SearchUniversities serves the htmx results for the university box.
func (h *Handlers) SearchUniversities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("uni_q")
	if len(q) > 100 {
		q = q[:100]
	}
	unis, err := h.Cat.SearchUniversities(r.Context(), q)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.OK(w, r, pages.UniSearchResults(unis))
}

// AdminList is the admin "all applications" view.
func (h *Handlers) AdminList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := ParseFilter(q)
	rows, st, page, err := h.Svc.Search(ctx, access.ScopeOf(reqctx.User(ctx)), f, model.ParsePage(q.Get("page")))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm := pages.ApplicationsVM{
		Rows:  rows,
		Stats: pages.AppStats{Total: st.Total, Waiting: st.Waiting, Admitted: st.Admitted, B2B: st.B2B},
		F: pages.AppFilterForm{
			Q: q.Get("q"), Branch: q["branch"], Country: q["country"], University: q["university"],
			Status: q["status"], Intake: q["intake"], Level: q["level"], Source: q["source"],
			Consultancy: q["consultancy"], From: q.Get("from"), To: q.Get("to"),
		},
		Page:        page,
		Query:       model.QueryWithout(q, "page"),
		DirectLimit: h.ExportDirectLimit,
	}
	for _, load := range []struct {
		dst *[]model.Option
		fn  func(context.Context) ([]model.Option, error)
	}{
		{&vm.Branches, h.Cat.BranchOptions}, {&vm.Countries, h.Cat.CountryOptions},
		{&vm.Universities, h.Cat.UniversityOptions}, {&vm.Consultancies, h.Cat.ConsultancyOptions},
		{&vm.Intakes, h.Svc.IntakeOptions},
	} {
		if *load.dst, err = load.fn(ctx); err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	web.OK(w, r, pages.Applications(vm))
}
