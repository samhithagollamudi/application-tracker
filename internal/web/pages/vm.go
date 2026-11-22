package pages

import (
	"fmt"

	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// ---- students list ----

type StudentFilterForm struct {
	Q, Branch, Consultancy, Country, University, Status string
}

type StudentsVM struct {
	Title, Subtitle string
	CanCreate       bool
	ShowBranch      bool
	ShowConsultancy bool
	Rows            []model.StudentRow
	Total           int
	F               StudentFilterForm
	Branches        []model.Option
	Consultancies   []model.Option
	Countries       []model.Option
	Universities    []model.Option
}

func (vm StudentsVM) Cols() int {
	n := 5
	if vm.ShowBranch {
		n++
	}
	if vm.ShowConsultancy {
		n++
	}
	return n
}

// ---- student detail ----

type StudentDetailVM struct {
	S        *model.Student
	Apps     []model.Application
	Activity []model.Activity
	CanMove  bool
	Branches []model.Option
	From     string // "students" or "applications" (breadcrumb)
}

// ---- student form ----

type StudentFormVM struct {
	Title, Sub, Action, CancelHref, Submit string
	F                                      model.StudentFields
	Errs                                   map[string]string
	DupHref                                string // link to the existing student when visible

	ShowConsultancy bool
	Consultancies   []model.Option
	ConsultancyID   string // selected id, or "__other"
	ConsOther       string

	ShowApp bool
	App     AppFormVM
}

// ---- application form ----

type AppFormVM struct {
	Title      string
	Action     string
	CancelHref string
	Submit     string
	F          model.ApplicationFields
	Errs       map[string]string
	Country    string // country of the picked list university (read-only box)
	Years      []model.Option
	Student    string // "Name · STU-12" subtitle on the standalone form
}

// ---- helpers ----

func studentHref(id int64) string { return fmt.Sprintf("/students/%d", id) }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ---- admin: applications ----

type AppFilterForm struct {
	Q, Branch, Country, University, Status, Intake, Level, Source, Consultancy, From, To string
}

func (f AppFilterForm) Active() int {
	n := 0
	for _, v := range []string{f.Q, f.Branch, f.Country, f.University, f.Status, f.Intake, f.Level, f.Source, f.Consultancy, f.From, f.To} {
		if v != "" {
			n++
		}
	}
	return n
}

type AppStats struct{ Total, Waiting, Admitted, B2B int }

type ApplicationsVM struct {
	Rows          []model.AppRow
	Stats         AppStats
	F             AppFilterForm
	Branches      []model.Option
	Countries     []model.Option
	Universities  []model.Option
	Intakes       []model.Option
	Consultancies []model.Option
	ExportHref    string
}

func studentFromApps(id int64) string { return fmt.Sprintf("/students/%d?from=applications", id) }

// ---- admin: catalog ----

type CatalogRow struct {
	ID      int64
	Name    string
	Country string
	AddedBy string // "" = admin
	Uses    int
}

type CatalogVM struct {
	Tab           string // "universities" | "consultancies"
	Universities  []CatalogRow
	Consultancies []CatalogRow
	Err           string
	Name          string
	Country       string
}

// ---- admin: users ----

type UserRow struct {
	ID     int64
	Name   string
	Email  string
	Role   model.Role
	Branch string
	Active bool
}

type NewPassword struct{ Name, Email, Password string }

type UserForm struct{ Name, Email, Role, Branch string }

type UsersVM struct {
	Users        []UserRow
	FreeBranches []model.Option
	NewPassword  *NewPassword
	Errs         map[string]string
	Form         UserForm
	DBSize       string
	MeID         int64
}
