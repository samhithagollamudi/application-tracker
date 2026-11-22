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
