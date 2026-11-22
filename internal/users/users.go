// Package users is the admin user management screen: create users with a
// generated password (shown once), reset passwords, deactivate/reactivate.
package users

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

type Handlers struct {
	Pool *pgxpool.Pool
	Auth *auth.Manager
}

func (h *Handlers) q() *gen.Queries { return gen.New(h.Pool) }

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", n/1024)
	}
}

func (h *Handlers) vm(r *http.Request) (pages.UsersVM, error) {
	ctx := r.Context()
	vm := pages.UsersVM{MeID: reqctx.User(ctx).ID, Errs: map[string]string{}}
	rows, err := h.q().ListUsers(ctx)
	if err != nil {
		return vm, err
	}
	for _, u := range rows {
		row := pages.UserRow{ID: u.ID, Name: u.Name, Email: u.Email, Role: model.Role(u.Role), Active: u.Active}
		if u.BranchName != nil {
			row.Branch = *u.BranchName
		}
		vm.Users = append(vm.Users, row)
	}
	free, err := h.q().FreeConsultantBranches(ctx)
	if err != nil {
		return vm, err
	}
	for _, b := range free {
		vm.FreeBranches = append(vm.FreeBranches, model.Option{Value: strconv.Itoa(int(b.ID)), Label: b.Name})
	}
	size, err := h.q().DatabaseSize(ctx)
	if err != nil {
		return vm, err
	}
	vm.DBSize = humanBytes(size)
	return vm, nil
}

// showPassword renders the page with a freshly generated password. The
// password is never stored in plain text or in the session; no-store keeps
// it out of browser caches.
func (h *Handlers) render(w http.ResponseWriter, r *http.Request, status int, vm pages.UsersVM) {
	w.Header().Set("Cache-Control", "no-store")
	web.Render(w, r, status, pages.Users(vm))
}

func (h *Handlers) Page(w http.ResponseWriter, r *http.Request) {
	vm, err := h.vm(r)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.render(w, r, http.StatusOK, vm)
}

func isUnique(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && (constraint == "" || pe.ConstraintName == constraint)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vm, err := h.vm(r)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	f := pages.UserForm{
		Name:   strings.Join(strings.Fields(r.PostFormValue("name")), " "),
		Email:  strings.TrimSpace(r.PostFormValue("email")),
		Role:   r.PostFormValue("role"),
		Branch: r.PostFormValue("branch_id"),
	}
	vm.Form = f
	role := model.Role(f.Role)
	switch {
	case f.Name == "":
		vm.Errs["name"] = "Enter the person’s name."
	case utf8.RuneCountInString(f.Name) > 100:
		vm.Errs["name"] = "Keep the name under 100 characters."
	}
	if a, err := mail.ParseAddress(f.Email); err != nil || a.Address != f.Email || len(f.Email) > 254 {
		vm.Errs["email"] = "Enter a valid email address."
	}
	if !role.Valid() {
		vm.Errs["role"] = "Pick a role."
	}
	var branchID *int16
	if role == model.RoleConsultant {
		n, err := strconv.ParseInt(f.Branch, 10, 16)
		if err != nil {
			vm.Errs["branch"] = "Pick a branch without an active consultant."
		} else {
			b := int16(n)
			branchID = &b
		}
	}
	if len(vm.Errs) == 0 {
		pw := auth.GeneratePassword()
		hash, err := auth.HashPassword(pw)
		if err != nil {
			web.ServerError(w, r, err)
			return
		}
		_, err = h.q().CreateUser(ctx, gen.CreateUserParams{Name: f.Name, Email: f.Email, Role: string(role), BranchID: branchID, PasswordHash: hash})
		switch {
		case isUnique(err, "users_email_key"):
			vm.Errs["email"] = "That user ID is already taken."
		case isUnique(err, "one_active_consultant_per_branch"):
			vm.Errs["branch"] = "That branch already has an active consultant. Deactivate them first."
		case err != nil:
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23503" { // unknown branch id
				vm.Errs["branch"] = "Pick a branch from the list."
				break
			}
			web.ServerError(w, r, err)
			return
		default:
			vm, err = h.vm(r)
			if err != nil {
				web.ServerError(w, r, err)
				return
			}
			vm.NewPassword = &pages.NewPassword{Name: f.Name, Email: f.Email, Password: pw}
			h.render(w, r, http.StatusOK, vm)
			return
		}
	}
	h.render(w, r, http.StatusUnprocessableEntity, vm)
}

func (h *Handlers) target(w http.ResponseWriter, r *http.Request) (*gen.GetUserByIDRow, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		web.NotFound(w, r)
		return nil, false
	}
	u, err := h.q().GetUserByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		web.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		web.ServerError(w, r, err)
		return nil, false
	}
	return &u, true
}

// Reset generates a new password. Bumping auth_version signs the user out everywhere.
func (h *Handlers) Reset(w http.ResponseWriter, r *http.Request) {
	u, ok := h.target(w, r)
	if !ok {
		return
	}
	pw := auth.GeneratePassword()
	hash, err := auth.HashPassword(pw)
	if err == nil {
		err = h.q().SetUserPassword(r.Context(), gen.SetUserPasswordParams{ID: u.ID, PasswordHash: hash})
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if u.ID == reqctx.User(r.Context()).ID {
		// Admin reset their own password: their session is now invalid.
		auth.Redirect(w, r, "/login")
		return
	}
	vm, err := h.vm(r)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm.NewPassword = &pages.NewPassword{Name: u.Name, Email: u.Email, Password: pw}
	h.render(w, r, http.StatusOK, vm)
}

func (h *Handlers) Toggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, ok := h.target(w, r)
	if !ok {
		return
	}
	if u.ID == reqctx.User(ctx).ID {
		h.Auth.Flash(ctx, "You can’t deactivate yourself.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	err := h.q().SetUserActive(ctx, gen.SetUserActiveParams{ID: u.ID, Active: !u.Active})
	switch {
	case isUnique(err, "one_active_consultant_per_branch"):
		h.Auth.Flash(ctx, deref(u.BranchName)+" already has an active consultant. Deactivate them first.")
	case err != nil:
		web.ServerError(w, r, err)
		return
	case u.Active:
		h.Auth.Flash(ctx, u.Name+" deactivated")
	default:
		h.Auth.Flash(ctx, u.Name+" reactivated")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
