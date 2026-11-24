package pages

import (
	"fmt"
	"time"

	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// ---- students list ----

// Filter forms hold the raw selected values (several per filter).
type StudentFilterForm struct {
	Q                                                                     string
	Branch, Consultancy, Country, University, Status, Level, Intake, Year []string
}

type StudentsVM struct {
	Title, Subtitle string
	CanCreate       bool
	ShowBranch      bool
	ShowConsultancy bool
	Rows            []model.StudentRow
	Page            model.Page
	Query           string // current filters as a query string, without "page"
	F               StudentFilterForm
	Branches        []model.Option
	Consultancies   []model.Option
	Countries       []model.Option
	Universities    []model.Option
	Years           []model.Option
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
	Countries  []string // the admin-maintained Country dropdown
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
	Q, From, To                                                                   string
	Branch, Country, University, Status, Intake, Year, Level, Source, Consultancy []string
}

// Active counts the filters in use (a multi-select counts once).
func (f AppFilterForm) Active() int {
	n := 0
	for _, v := range []string{f.Q, f.From, f.To} {
		if v != "" {
			n++
		}
	}
	for _, v := range [][]string{f.Branch, f.Country, f.University, f.Status, f.Intake, f.Year, f.Level, f.Source, f.Consultancy} {
		if len(v) > 0 {
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
	Years         []model.Option
	Consultancies []model.Option
	Page          model.Page
	Query         string // current filters as a query string, without "page"
	DirectLimit   int    // larger exports run in the background
}

func studentFromApps(id int64) string { return fmt.Sprintf("/students/%d?from=applications", id) }

// ---- admin: catalog ----

type CatalogRow struct {
	OneOff  int
	ID      int64
	Name    string
	Country string
	AddedBy string // "" = admin
	Uses    int
}

type CatalogVM struct {
	Tab           string // "universities" | "consultancies" | "countries"
	Universities  []CatalogRow
	Consultancies []CatalogRow
	Countries     []CatalogRow // Uses = universities in that country, OneOff = one-off applications
	CountryNames  []string     // for the add-university dropdown
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

// ---- admin: exports ----

type ExportRow struct {
	ID       int64
	Status   string // queued | running | done | failed
	Total    int
	Done     int
	FileName string
	Size     int64
	Error    string
	Created  time.Time
	Query    string
}

func (e ExportRow) Percent() int {
	if e.Total == 0 {
		return 0
	}
	return e.Done * 100 / e.Total
}

type ExportsVM struct {
	Rows       []ExportRow
	AnyPending bool
}

type ExportNotice struct {
	ID   int64
	OK   bool
	Rows int
}
