package model

import (
	"strconv"
	"strings"
	"time"
)

// ApplicationFields are the form fields, kept as strings so a form can be
// re-rendered exactly as typed when validation fails.
type ApplicationFields struct {
	UniversityID      string // id from the admin list, or ""
	UniversityName    string // display name for the search box when UniversityID is set
	UniOther          string // one-off university name ("Other")
	CountryName       string // the Country dropdown: the list university's country, or the one-off university's
	Level             string
	Programme         string
	Priority          string
	Intake            string
	Year              string
	ApplicationNumber string
	SubmissionDate    string // YYYY-MM-DD
	Status            string
	PersonalCode      string
	TuitionFee        string
	Currency          string
	TuitionPer        string
	Scholarship       string
}

// IsEmpty reports whether nothing at all was entered (used to skip the
// optional first application on the new-student form).
func (f ApplicationFields) IsEmpty() bool {
	for _, v := range []string{f.UniversityID, f.UniOther, f.Level, f.Programme, f.Priority,
		f.Intake, f.ApplicationNumber, f.SubmissionDate, f.Status, f.PersonalCode, f.TuitionFee,
		f.TuitionPer, f.Scholarship} {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// HasAfterAdmission reports whether any after-admission field is filled.
func (f ApplicationFields) HasAfterAdmission() bool {
	return f.PersonalCode != "" || f.TuitionFee != "" || f.TuitionPer != "" || f.Scholarship != ""
}

type Application struct {
	ID        int64
	StudentID int64
	Uni       string // university display name (list or one-off)
	Country   string
	Custom    bool // one-off university
	CreatedAt time.Time
	UpdatedAt time.Time
	ApplicationFields
}

func (a *Application) Code() string { return ApplicationCode(a.ID) }

// Tuition renders e.g. "11,000 EUR per year" (amount, currency, per).
func (a *Application) Tuition() string {
	if a.TuitionFee == "" {
		return ""
	}
	s := Amount(a.TuitionFee)
	if a.Currency != "" {
		s += " " + a.Currency
	}
	if a.TuitionPer != "" {
		s += " per " + strings.ToLower(PerLabel(a.TuitionPer))
	}
	return s
}

// Amount formats a stored decimal like "11000.00" as "11,000" and "9500.50" as "9,500.50".
func Amount(v string) string {
	whole, frac, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(whole)
	if err != nil {
		return v
	}
	s := Thousands(n)
	if strings.Trim(frac, "0") != "" {
		s += "." + frac
	}
	return s
}

// AppRow is one line of the admin applications table.
type AppRow struct {
	ID              int64
	StudentID       int64
	FirstName       string
	FamilyName      string
	Email           string
	Uni             string
	Country         string
	Custom          bool
	Programme       string
	Level           string
	Intake          string
	Year            string
	BranchName      string
	ConsultancyName string
	OwnerName       string
	Status          string
	UpdatedAt       time.Time
}

func (r *AppRow) Code() string        { return ApplicationCode(r.ID) }
func (r *AppRow) StudentName() string { return FullName(r.FirstName, r.FamilyName) }
