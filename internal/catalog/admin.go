package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

// AdminHandlers manage the university and consultancy lists.
type AdminHandlers struct {
	Svc  *Service
	Auth *auth.Manager
}

func addedBy(name, role *string) string {
	if name == nil || role == nil || *role == string(model.RoleAdmin) {
		return ""
	}
	return *name
}

func (h *AdminHandlers) vm(r *http.Request, tab string) (pages.CatalogVM, error) {
	ctx := r.Context()
	vm := pages.CatalogVM{Tab: tab}
	us, err := h.Svc.q.ListUniversitiesAdmin(ctx)
	if err != nil {
		return vm, err
	}
	for _, u := range us {
		vm.Universities = append(vm.Universities, pages.CatalogRow{ID: u.ID, Name: u.Name, Country: u.Country,
			AddedBy: addedBy(u.AddedByName, u.AddedByRole), Uses: int(u.Uses)})
	}
	cs, err := h.Svc.q.ListConsultanciesAdmin(ctx)
	if err != nil {
		return vm, err
	}
	for _, c := range cs {
		vm.Consultancies = append(vm.Consultancies, pages.CatalogRow{ID: c.ID, Name: c.Name,
			AddedBy: addedBy(c.AddedByName, c.AddedByRole), Uses: int(c.Uses)})
	}
	return vm, nil
}

func tabOf(r *http.Request) string {
	if r.URL.Query().Get("tab") == "consultancies" || r.PathValue("kind") == "consultancies" {
		return "consultancies"
	}
	return "universities"
}

func (h *AdminHandlers) Page(w http.ResponseWriter, r *http.Request) {
	vm, err := h.vm(r, tabOf(r))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.OK(w, r, pages.Catalog(vm))
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (h *AdminHandlers) Add(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tab := tabOf(r)
	if k := r.PathValue("kind"); k != "universities" && k != "consultancies" {
		web.NotFound(w, r)
		return
	}
	name := strings.Join(strings.Fields(r.PostFormValue("name")), " ")
	country := r.PostFormValue("country")
	actor := reqctx.User(ctx).ID

	msg := ""
	switch {
	case name == "":
		msg = "Enter a name."
	case utf8.RuneCountInString(name) > 200:
		msg = "Keep the name under 200 characters."
	case tab == "universities" && !validCountry(country):
		msg = "Pick a country from the list."
	}
	var err error
	if msg == "" {
		if tab == "universities" {
			_, err = h.Svc.q.CreateUniversity(ctx, gen.CreateUniversityParams{Name: name, Country: country, CreatedBy: &actor})
		} else {
			_, err = h.Svc.q.CreateConsultancy(ctx, gen.CreateConsultancyParams{Name: name, CreatedBy: &actor})
		}
		if isUnique(err) {
			msg, err = "“"+name+"” is already in the list.", nil
		}
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if msg != "" {
		vm, err := h.vm(r, tab)
		if err != nil {
			web.ServerError(w, r, err)
			return
		}
		vm.Err, vm.Name, vm.Country = msg, name, country
		web.Render(w, r, http.StatusUnprocessableEntity, pages.Catalog(vm))
		return
	}
	h.Auth.Flash(ctx, "Added “"+name+"”")
	http.Redirect(w, r, "/admin/catalog?tab="+tab, http.StatusSeeOther)
}

// Delete removes an entry only while nothing uses it (no merging/reassigning).
func (h *AdminHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		web.NotFound(w, r)
		return
	}
	var n int64
	switch tabOf(r) {
	case "universities":
		n, err = h.Svc.q.DeleteUnusedUniversity(ctx, id)
	default:
		n, err = h.Svc.q.DeleteUnusedConsultancy(ctx, id)
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if n == 0 {
		h.Auth.Flash(ctx, "Not removed: it doesn’t exist or is in use.")
	} else {
		h.Auth.Flash(ctx, "Removed")
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/catalog?tab=%s", tabOf(r)), http.StatusSeeOther)
}

func validCountry(c string) bool {
	for _, x := range model.Countries {
		if x == c {
			return true
		}
	}
	return false
}
