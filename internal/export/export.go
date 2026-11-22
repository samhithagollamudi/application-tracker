// Package export streams the admin's filtered applications as CSV, one row
// per application, columns following the form.
package export

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/httpx"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
)

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

type Handler struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// safe stops spreadsheet apps from treating a cell as a formula.
func safe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func (h *Handler) CSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := reqctx.User(ctx)
	f := applications.ParseFilter(r.URL.Query())
	var args access.Args
	sql := `SELECT ` + columns + applications.FromSQL + ` WHERE ` + applications.Where(access.ScopeOf(u), f, &args) +
		` ORDER BY a.updated_at DESC, a.id DESC`
	rows, err := h.Pool.Query(ctx, sql, args.Values()...)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	defer rows.Close()

	name := "applications-" + time.Now().In(model.IST).Format("2006-01-02") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\uFEFF")) // BOM so Excel reads UTF-8 names correctly

	rc := http.NewResponseController(w)
	cw := csv.NewWriter(w)
	_ = cw.Write(header)

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
			h.abort(r, err)
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
			return // client went away
		}
		n++
		if n%500 == 0 {
			cw.Flush()
			_ = rc.Flush()
		}
	}
	if err := rows.Err(); err != nil {
		h.abort(r, err)
	}
	cw.Flush()
	h.Log.Info("csv export", "id", httpx.RequestID(ctx), "user_id", u.ID, "rows", n)
}

// abort kills the connection mid-stream so a failed export can't be
// mistaken for a complete (but truncated) file.
func (h *Handler) abort(r *http.Request, err error) {
	h.Log.Error("csv export failed", "id", httpx.RequestID(r.Context()), "err", err)
	panic(http.ErrAbortHandler)
}
