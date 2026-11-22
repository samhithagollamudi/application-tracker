package students

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/activity"
	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

type Handlers struct {
	Svc  *Service
	Cat  *catalog.Service
	Apps *applications.Service
	Pool *pgxpool.Pool
	Auth *auth.Manager
}

// PathID parses the {id} path value; ok=false means respond 404.
func PathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	u := reqctx.User(r.Context())
	scope := access.ScopeOf(u)
	q := r.URL.Query()
	ff := pages.StudentFilterForm{
		Q: q.Get("q"), Branch: q.Get("branch"), Consultancy: q.Get("consultancy"),
		Country: q.Get("country"), University: q.Get("university"), Status: q.Get("status"),
	}
	f := ListFilter{Q: ff.Q, Country: ff.Country}
	if n, err := strconv.ParseInt(ff.Branch, 10, 16); err == nil {
		f.BranchID = int16(n)
	}
	f.ConsultancyID, _ = strconv.ParseInt(ff.Consultancy, 10, 64)
	f.UniversityID, _ = strconv.ParseInt(ff.University, 10, 64)
	if model.ValidOption(model.Statuses, ff.Status) {
		f.Status = ff.Status
	}

	rows, total, err := h.Svc.List(r.Context(), scope, f)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm := pages.StudentsVM{Rows: rows, Total: total, F: ff, CanCreate: scope.CanCreateStudents()}
	switch u.Role {
	case model.RoleAdmin:
		vm.Title, vm.Subtitle = "Students", "Every student across all branches, internal and B2B."
		vm.ShowBranch, vm.ShowConsultancy = true, true
	case model.RoleConsultant:
		vm.Title, vm.Subtitle = "My students", "All students of the "+u.BranchName+" branch. Open one to see and update their applications."
	default:
		vm.Title, vm.Subtitle = "Students", "All students referred by other consultancies (B2B). Branch consultants can’t see these, and you can’t see theirs."
		vm.ShowConsultancy = true
	}
	ctx := r.Context()
	if vm.ShowBranch {
		if vm.Branches, err = h.Cat.BranchOptions(ctx); err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	if vm.ShowConsultancy {
		if vm.Consultancies, err = h.Cat.ConsultancyOptions(ctx); err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	if vm.Countries, err = h.Cat.CountryOptions(ctx); err != nil {
		web.ServerError(w, r, err)
		return
	}
	if vm.Universities, err = h.Cat.UniversityOptions(ctx); err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.OK(w, r, pages.Students(vm))
}

func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	u := reqctx.User(r.Context())
	s, err := h.Svc.Get(r.Context(), access.ScopeOf(u), id)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm := pages.StudentDetailVM{S: s, CanMove: u.IsAdmin(), From: r.URL.Query().Get("from")}
	if vm.Apps, err = applications.ListForStudent(r.Context(), h.Pool, id); err != nil {
		web.ServerError(w, r, err)
		return
	}
	if vm.Activity, err = activity.List(r.Context(), h.Pool, id); err != nil {
		web.ServerError(w, r, err)
		return
	}
	if vm.CanMove {
		if vm.Branches, err = h.Cat.BranchOptions(r.Context()); err != nil {
			web.ServerError(w, r, err)
			return
		}
	}
	web.OK(w, r, pages.StudentDetail(vm))
}

func parseStudentFields(r *http.Request) model.StudentFields {
	v := r.PostFormValue
	return model.StudentFields{
		FirstName: v("first_name"), FamilyName: v("family_name"), Gender: v("gender"),
		DateOfBirth: v("date_of_birth"), Email: v("email"), Address: v("address"),
		BAUniversity: v("ba_university"), BASpecialisation: v("ba_specialisation"), BADuration: v("ba_duration"), BAGrade: v("ba_grade"),
		MAUniversity: v("ma_university"), MASpecialisation: v("ma_specialisation"), MADuration: v("ma_duration"), MAGrade: v("ma_grade"),
		XIIBoard: v("xii_board"), XIISubjects: v("xii_subjects"), XIIGrade: v("xii_grade"),
	}
}

func parseConsultancy(r *http.Request) (ConsultancyChoice, string, string) {
	sel := r.PostFormValue("consultancy_id")
	other := r.PostFormValue("consultancy_other")
	if sel == "__other" {
		return ConsultancyChoice{OtherName: other}, sel, other
	}
	id, _ := strconv.ParseInt(sel, 10, 64)
	return ConsultancyChoice{ID: id}, sel, ""
}

func (h *Handlers) newFormVM(r *http.Request, u *model.User) (pages.StudentFormVM, error) {
	vm := pages.StudentFormVM{
		Title: "New student & application", Action: "/students/new", CancelHref: "/students", Submit: "Save student",
		ShowApp: true, App: h.Apps.FormVM(model.ApplicationFields{}, nil, ""),
	}
	if u.Role == model.RoleAgentNetwork {
		vm.Sub = "Only the application email is required. The student is saved as a B2B student, so branch consultants can’t see them."
		vm.ShowConsultancy = true
		var err error
		if vm.Consultancies, err = h.Cat.ConsultancyOptions(r.Context()); err != nil {
			return vm, err
		}
	} else {
		vm.Sub = "Only the application email is required. The student is saved to the " + u.BranchName + " branch."
	}
	return vm, nil
}

func (h *Handlers) NewForm(w http.ResponseWriter, r *http.Request) {
	vm, err := h.newFormVM(r, reqctx.User(r.Context()))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.OK(w, r, pages.StudentForm(vm))
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := reqctx.User(ctx)
	vm, err := h.newFormVM(r, u)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm.F = parseStudentFields(r)
	cons, sel, other := parseConsultancy(r)
	vm.ConsultancyID, vm.ConsOther = sel, other

	appF := applications.ParseFields(r)
	var appIn *applications.Input
	errs := FieldErrors{}
	if !appF.IsEmpty() {
		in, aerrs, err := h.Apps.Resolve(ctx, &appF)
		if err != nil {
			web.ServerError(w, r, err)
			return
		}
		for k, v := range aerrs {
			errs[k] = v
		}
		appIn = in
	}
	country := ""
	if appIn != nil {
		country = appIn.Country
	}
	vm.App = h.Apps.FormVM(appF, errs, country)

	var id int64
	if len(errs) == 0 {
		id, err = h.Svc.Create(ctx, u, vm.F, cons, func(ctx2 context.Context, tx pgx.Tx, sid int64) error {
			if appIn == nil {
				return nil
			}
			_, err := h.Apps.Insert(ctx2, tx, u.ID, sid, appIn)
			return err
		})
	} else {
		// Still validate the student fields so all errors show at once.
		if serrs := Validate(&vm.F); serrs != nil {
			for k, v := range serrs {
				errs[k] = v
			}
		}
		err = errs
	}
	if err != nil {
		if h.formError(w, r, err, &vm) {
			return
		}
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(ctx, "Student saved")
	http.Redirect(w, r, fmt.Sprintf("/students/%d", id), http.StatusSeeOther)
}

// formError renders the form again for validation/duplicate errors. It
// returns false for unexpected errors, which the caller turns into a 500.
func (h *Handlers) formError(w http.ResponseWriter, r *http.Request, err error, vm *pages.StudentFormVM) bool {
	var fe FieldErrors
	var de *DuplicateError
	switch {
	case errors.As(err, &fe):
		if vm.Errs == nil {
			vm.Errs = map[string]string{}
		}
		for k, v := range fe {
			vm.Errs[k] = v
		}
		// Application field errors live on the embedded app form too.
		vm.App.Errs = vm.Errs
	case errors.As(err, &de):
		vm.Errs = map[string]string{}
		vm.App.Errs = vm.Errs
		if de.Visible {
			vm.Errs["email"] = "A student with this email already exists."
			vm.DupHref = fmt.Sprintf("/students/%d", de.ExistingID)
		} else {
			vm.Errs["email"] = "This email is already registered. Contact the admin."
		}
	default:
		return false
	}
	web.Render(w, r, http.StatusUnprocessableEntity, pages.StudentForm(*vm))
	return true
}

func (h *Handlers) editVM(r *http.Request, u *model.User, s *model.Student) (pages.StudentFormVM, error) {
	vm := pages.StudentFormVM{
		Title: "Edit " + s.Name(), Action: fmt.Sprintf("/students/%d/edit", s.ID),
		CancelHref: fmt.Sprintf("/students/%d", s.ID), Submit: "Save changes",
		F: s.StudentFields,
	}
	vm.Sub = s.Code() + " · "
	if s.Source == "b2b" {
		vm.Sub += "B2B"
	} else {
		vm.Sub += s.BranchName + " branch"
	}
	if s.Source == "b2b" && u.Role != model.RoleConsultant {
		vm.ShowConsultancy = true
		if s.ConsultancyID != nil {
			vm.ConsultancyID = strconv.FormatInt(*s.ConsultancyID, 10)
		}
		var err error
		if vm.Consultancies, err = h.Cat.ConsultancyOptions(r.Context()); err != nil {
			return vm, err
		}
	}
	return vm, nil
}

func (h *Handlers) loadForEdit(w http.ResponseWriter, r *http.Request) (*model.User, *model.Student, bool) {
	id, ok := PathID(r)
	if !ok {
		web.NotFound(w, r)
		return nil, nil, false
	}
	u := reqctx.User(r.Context())
	s, err := h.Svc.Get(r.Context(), access.ScopeOf(u), id)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return nil, nil, false
	}
	if err != nil {
		web.ServerError(w, r, err)
		return nil, nil, false
	}
	return u, s, true
}

func (h *Handlers) EditForm(w http.ResponseWriter, r *http.Request) {
	u, s, ok := h.loadForEdit(w, r)
	if !ok {
		return
	}
	vm, err := h.editVM(r, u, s)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.OK(w, r, pages.StudentForm(vm))
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	u, s, ok := h.loadForEdit(w, r)
	if !ok {
		return
	}
	vm, err := h.editVM(r, u, s)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm.F = parseStudentFields(r)
	var cons *ConsultancyChoice
	if vm.ShowConsultancy {
		c, sel, other := parseConsultancy(r)
		vm.ConsultancyID, vm.ConsOther = sel, other
		cons = &c
	}
	err = h.Svc.Update(r.Context(), u, s.ID, vm.F, cons)
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		if h.formError(w, r, err, &vm) {
			return
		}
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(r.Context(), "Changes saved")
	http.Redirect(w, r, fmt.Sprintf("/students/%d", s.ID), http.StatusSeeOther)
}

func (h *Handlers) Move(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(r)
	if !ok {
		web.NotFound(w, r)
		return
	}
	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 16)
	if err != nil {
		web.Error(w, r, http.StatusBadRequest)
		return
	}
	msg, err := h.Svc.Move(r.Context(), reqctx.User(r.Context()), id, int16(branchID))
	if errors.Is(err, ErrNotFound) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.Auth.Flash(r.Context(), msg)
	http.Redirect(w, r, fmt.Sprintf("/students/%d", id), http.StatusSeeOther)
}
