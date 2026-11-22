// Package export produces the admin's filtered applications as CSV, one row
// per application, columns following the form.
//
// Small exports (up to DirectLimit rows) stream straight to the browser.
// Larger ones run as background jobs (see runner.go): the admin keeps working
// and gets a notice with a download link when the file is ready.
package export

import (
	"context"
	"encoding/csv"
	"io"
	"strings"
	"time"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// DefaultDirectLimit is the largest export streamed immediately.
const DefaultDirectLimit = 5000

var header = []string{
	"Application ID", "Branch", "Owner", "Consultancy",
	"Student ID", "First name", "Family name", "Gender", "Date of birth", "Application email", "Address (as per passport)",
	"Bachelor's university", "Bachelor's specialisation", "Bachelor's duration", "Bachelor's CGPA/%",
	"Master's university", "Master's specialisation", "Master's duration", "Master's CGPA/%",
	"12th board", "12th subjects", "12th CGPA/%",
	"University", "Country", "One-off university", "Programme level", "Programme", "Priority number", "Intake", "Year",
	"Application number", "Submission date", "Status",
	"Personal code number", "Tuition fee", "Currency", "Tuition fee per", "Scholarship",
	"Created on", "Last updated",
}

// columns must line up with header.
const columns = `a.id, coalesce(b.name, ''), o.name, coalesce(c.name, ''),
	s.id, coalesce(s.first_name, ''), coalesce(s.family_name, ''), coalesce(s.gender, ''),
	coalesce(to_char(s.date_of_birth, 'YYYY-MM-DD'), ''), s.email::text, coalesce(s.address, ''),
	coalesce(s.ba_university, ''), coalesce(s.ba_specialisation, ''), coalesce(s.ba_duration, ''), coalesce(s.ba_grade, ''),
	coalesce(s.ma_university, ''), coalesce(s.ma_specialisation, ''), coalesce(s.ma_duration, ''), coalesce(s.ma_grade, ''),
	coalesce(s.xii_board, ''), coalesce(s.xii_subjects, ''), coalesce(s.xii_grade, ''),
	coalesce(u.name, a.custom_university_name, ''), coalesce(u.country, a.custom_university_country, ''), a.university_id IS NULL,
	coalesce(a.programme_level, ''), coalesce(a.programme, ''), coalesce(a.priority::text, ''), coalesce(a.intake, ''), coalesce(a.year::text, ''),
	coalesce(a.application_number, ''), coalesce(to_char(a.submission_date, 'YYYY-MM-DD'), ''), coalesce(a.status, ''),
	coalesce(a.personal_code, ''), coalesce(a.tuition_fee::text, ''), coalesce(a.tuition_currency, ''), coalesce(a.tuition_per, ''), coalesce(a.scholarship, ''),
	a.created_at, a.updated_at`

// safe stops spreadsheet apps from treating a cell as a formula.
func safe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// FileName is the download name, dated in IST.
func FileName(t time.Time) string {
	return "applications-" + t.In(model.IST).Format("2006-01-02") + ".csv"
}

// Write streams the matching applications as CSV (UTF-8 BOM + header + rows)
// in a single query, so memory stays flat whatever the row count. flush is
// called every 500 rows (may be nil); progress after every row (may be nil).
func Write(ctx context.Context, db gen.DBTX, scope access.Scope, f applications.Filter, w io.Writer,
	flush func(), progress func(n int)) (int, error) {
	var args access.Args
	sql := `SELECT ` + columns + applications.FromSQL + ` WHERE ` + applications.Where(scope, f, &args) +
		` ORDER BY a.updated_at DESC, a.id DESC`
	rows, err := db.Query(ctx, sql, args.Values()...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if _, err := w.Write([]byte("\uFEFF")); err != nil { // BOM so Excel reads UTF-8 names correctly
		return 0, err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return 0, err
	}

	n := 0
	for rows.Next() {
		var (
			appID, studentID                                    int64
			custom                                              bool
			created, updated                                    time.Time
			branch, owner, cons, first, family, gender, dob     string
			email, addr, bau, bas, bad, bag, mau, mas, mad, mag string
			xb, xs, xg, uni, country, level, prog, prio, intake string
			year, appNo, sub, status, pcode, fee, cur, per, sch string
		)
		if err := rows.Scan(&appID, &branch, &owner, &cons, &studentID, &first, &family, &gender, &dob, &email, &addr,
			&bau, &bas, &bad, &bag, &mau, &mas, &mad, &mag, &xb, &xs, &xg,
			&uni, &country, &custom, &level, &prog, &prio, &intake, &year, &appNo, &sub, &status,
			&pcode, &fee, &cur, &per, &sch, &created, &updated); err != nil {
			return n, err
		}
		oneOff, statusLabel := "", ""
		if custom {
			oneOff = "Yes"
		}
		if status != "" {
			statusLabel = model.StatusLabel(status)
		}
		rec := []string{
			model.ApplicationCode(appID), branch, owner, cons,
			model.StudentCode(studentID), first, family, gender, dob, email, addr,
			bau, bas, bad, bag, mau, mas, mad, mag, xb, xs, xg,
			uni, country, oneOff, model.LevelLabel(level), prog, prio, model.IntakeLabel(intake), year,
			appNo, sub, statusLabel,
			pcode, fee, cur, model.PerLabel(per), sch,
			created.In(model.IST).Format("2006-01-02"), updated.In(model.IST).Format("2006-01-02"),
		}
		for i := range rec {
			rec[i] = safe(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return n, err
		}
		n++
		if progress != nil {
			progress(n)
		}
		if n%500 == 0 {
			cw.Flush()
			if err := cw.Error(); err != nil {
				return n, err
			}
			if flush != nil {
				flush()
			}
		}
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	cw.Flush()
	return n, cw.Error()
}
