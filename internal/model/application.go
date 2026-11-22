package model

import (
	"strings"
	"time"
)

// ApplicationFields are the form fields, kept as strings so a form can be
// re-rendered exactly as typed when validation fails.
type ApplicationFields struct {
	UniversityID      string // id from the admin list, or ""
	UniversityName    string // display name for the search box when UniversityID is set
	UniOther          string // one-off university name ("Other")
	UniOtherCountry   string
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
	for _, v := range []string{f.UniversityID, f.UniOther, f.UniOtherCountry, f.Level, f.Programme, f.Priority,
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

// Tuition renders e.g. "SEK 120000 per year".
func (a *Application) Tuition() string {
	if a.TuitionFee == "" {
		return ""
	}
	s := a.TuitionFee
	if a.Currency != "" {
		s = a.Currency + " " + s
	}
	if a.TuitionPer != "" {
		s += " per " + strings.ToLower(PerLabel(a.TuitionPer))
	}
	return s
}
