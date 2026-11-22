package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// StudentCode is the human ID shown everywhere ("STU-1042").
func StudentCode(id int64) string { return fmt.Sprintf("STU-%d", id) }

// ApplicationCode is the human ID for an application ("APP-2017").
func ApplicationCode(id int64) string { return fmt.Sprintf("APP-%d", id) }

// ParseCode accepts "STU-12", "stu12", "APP-12" or "12" and returns the number.
func ParseCode(s, prefix string) (int64, bool) {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimPrefix(s, prefix)
	s = strings.TrimPrefix(s, "-")
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n > 0
}

func FullName(first, family string) string {
	n := strings.TrimSpace(first + " " + family)
	if n == "" {
		return "(no name)"
	}
	return n
}

// StudentFields are the editable form fields (same for every role).
type StudentFields struct {
	FirstName, FamilyName, Gender, DateOfBirth, Email, Address string
	BAUniversity, BASpecialisation, BADuration, BAGrade        string
	MAUniversity, MASpecialisation, MADuration, MAGrade        string
	XIIBoard, XIISubjects, XIIGrade                            string
}

type Student struct {
	ID              int64
	Source          string // internal | b2b
	BranchID        *int16
	BranchName      string
	ConsultancyID   *int64
	ConsultancyName string
	OwnerID         int64
	OwnerName       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StudentFields
}

func (s *Student) Code() string { return StudentCode(s.ID) }
func (s *Student) Name() string { return FullName(s.FirstName, s.FamilyName) }

// StudentRow is one line of the students list.
type StudentRow struct {
	ID              int64
	FirstName       string
	FamilyName      string
	Email           string
	Source          string
	BranchName      string
	ConsultancyName string
	AppCount        int
	Universities    string
	LatestStatus    *string // nil when there are no applications
	UpdatedAt       time.Time
}

func (s *StudentRow) Code() string { return StudentCode(s.ID) }
func (s *StudentRow) Name() string { return FullName(s.FirstName, s.FamilyName) }

type Activity struct {
	Text      string
	ActorName string
	At        time.Time
}

// IST is the business timezone (all branches are in India).
var IST = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+1800)
	}
	return loc
}()

// FormatDate renders dates like "05 Mar 2026" in IST; zero time renders "".
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(IST).Format("02 Jan 2006")
}

// FormatISODate turns "2026-03-05" into "05 Mar 2026" (passes through anything else).
func FormatISODate(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return FormatDate(t)
}
