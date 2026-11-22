package applications

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// Filter is the admin applications filter set, parsed from the URL so views
// are bookmarkable and the CSV export uses exactly the same rows.
type Filter struct {
	Q             string
	BranchID      int16
	Country       string
	UniversityID  int64
	Status        string
	Intake        string // "autumn"/"spring"
	Year          int16
	Level         string
	Source        string // internal | b2b
	ConsultancyID int64
	NoConsultancy bool
	From, To      string // YYYY-MM-DD, created date (IST), inclusive
}

// ParseFilter reads a Filter from query values, ignoring invalid entries.
func ParseFilter(q url.Values) Filter {
	f := Filter{Q: strings.TrimSpace(q.Get("q")), Country: q.Get("country")}
	if n, err := strconv.ParseInt(q.Get("branch"), 10, 16); err == nil {
		f.BranchID = int16(n)
	}
	f.UniversityID, _ = strconv.ParseInt(q.Get("university"), 10, 64)
	if s := q.Get("status"); model.ValidOption(model.Statuses, s) {
		f.Status = s
	}
	if in, y, ok := strings.Cut(q.Get("intake"), "-"); ok && model.ValidOption(model.Intakes, in) {
		if n, err := strconv.ParseInt(y, 10, 16); err == nil {
			f.Intake, f.Year = in, int16(n)
		}
	}
	if l := q.Get("level"); model.ValidOption(model.Levels, l) {
		f.Level = l
	}
	if s := q.Get("source"); s == "internal" || s == "b2b" {
		f.Source = s
	}
	switch c := q.Get("consultancy"); c {
	case "none":
		f.NoConsultancy = true
	default:
		f.ConsultancyID, _ = strconv.ParseInt(c, 10, 64)
	}
	if d := q.Get("from"); validDate(d) {
		f.From = d
	}
	if d := q.Get("to"); validDate(d) {
		f.To = d
	}
	return f
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// FromSQL and Where are exported for the CSV export, which must select
// exactly the rows the admin view shows.
const FromSQL = fromSQL

func Where(scope access.Scope, f Filter, args *access.Args) string { return where(scope, f, args) }

// fromSQL joins applications (a) to students (s), universities (u),
// branches (b), consultancies (c) and owners (o).
const fromSQL = ` FROM applications a
	JOIN students s ON s.id = a.student_id
	LEFT JOIN universities u ON u.id = a.university_id
	LEFT JOIN branches b ON b.id = s.branch_id
	LEFT JOIN consultancies c ON c.id = s.consultancy_id
	JOIN users o ON o.id = s.owner_id`

// where builds the WHERE clause: scope first, then the filters.
func where(scope access.Scope, f Filter, args *access.Args) string {
	w := []string{access.StudentFilter(scope, "s", args)}
	if f.Q != "" {
		p := args.Add("%" + escapeLike(strings.ToLower(f.Q)) + "%")
		cond := `(lower(coalesce(s.first_name,'') || ' ' || coalesce(s.family_name,'') || ' ' || s.email::text) LIKE ` + p +
			` OR lower(coalesce(a.application_number, '')) LIKE ` + p +
			` OR lower(coalesce(u.name, a.custom_university_name)) LIKE ` + p
		if id, ok := model.ParseCode(f.Q, "APP"); ok {
			cond += ` OR a.id = ` + args.Add(id)
		}
		if id, ok := model.ParseCode(f.Q, "STU"); ok {
			cond += ` OR s.id = ` + args.Add(id)
		}
		w = append(w, cond+`)`)
	}
	if f.BranchID != 0 {
		w = append(w, `s.branch_id = `+args.Add(f.BranchID))
	}
	if f.Country != "" {
		w = append(w, `coalesce(u.country, a.custom_university_country) = `+args.Add(f.Country))
	}
	if f.UniversityID != 0 {
		w = append(w, `a.university_id = `+args.Add(f.UniversityID))
	}
	if f.Status != "" {
		w = append(w, `a.status = `+args.Add(f.Status))
	}
	if f.Intake != "" {
		w = append(w, `a.intake = `+args.Add(f.Intake)+` AND a.year = `+args.Add(f.Year))
	}
	if f.Level != "" {
		w = append(w, `a.programme_level = `+args.Add(f.Level))
	}
	if f.Source != "" {
		w = append(w, `s.source = `+args.Add(f.Source))
	}
	if f.NoConsultancy {
		w = append(w, `s.consultancy_id IS NULL`)
	} else if f.ConsultancyID != 0 {
		w = append(w, `s.consultancy_id = `+args.Add(f.ConsultancyID))
	}
	if f.From != "" {
		w = append(w, `(a.created_at AT TIME ZONE 'Asia/Kolkata')::date >= `+args.Add(f.From)+`::date`)
	}
	if f.To != "" {
		w = append(w, `(a.created_at AT TIME ZONE 'Asia/Kolkata')::date <= `+args.Add(f.To)+`::date`)
	}
	return strings.Join(w, " AND ")
}

type Stats struct {
	Total, Waiting, Admitted, B2B int
}

const pageLimit = 200

// Search returns up to 200 rows (most recently updated first) and stats over all matches.
func (s *Service) Search(ctx context.Context, scope access.Scope, f Filter) ([]model.AppRow, Stats, error) {
	var st Stats
	var args access.Args
	w := where(scope, f, &args)
	err := s.pool.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE a.status IN ('applied', 'reserve')),
		count(*) FILTER (WHERE a.status IN ('admitted', 'fees_paid', 'visa_granted', 'visa_rejected')),
		count(*) FILTER (WHERE s.source = 'b2b')`+fromSQL+` WHERE `+w, args.Values()...).
		Scan(&st.Total, &st.Waiting, &st.Admitted, &st.B2B)
	if err != nil {
		return nil, st, err
	}

	args = access.Args{}
	w = where(scope, f, &args)
	rows, err := s.pool.Query(ctx, `SELECT a.id, s.id, coalesce(s.first_name,''), coalesce(s.family_name,''), s.email::text,
		coalesce(u.name, a.custom_university_name, ''), coalesce(u.country, a.custom_university_country, ''), a.university_id IS NULL,
		coalesce(a.programme, ''), coalesce(a.programme_level, ''), coalesce(a.intake, ''), coalesce(a.year::text, ''),
		coalesce(b.name, ''), coalesce(c.name, ''), o.name, coalesce(a.status, ''), a.updated_at`+fromSQL+
		` WHERE `+w+` ORDER BY a.updated_at DESC, a.id DESC LIMIT `+strconv.Itoa(pageLimit), args.Values()...)
	if err != nil {
		return nil, st, err
	}
	defer rows.Close()
	var out []model.AppRow
	for rows.Next() {
		var r model.AppRow
		if err := rows.Scan(&r.ID, &r.StudentID, &r.FirstName, &r.FamilyName, &r.Email, &r.Uni, &r.Country, &r.Custom,
			&r.Programme, &r.Level, &r.Intake, &r.Year, &r.BranchName, &r.ConsultancyName, &r.OwnerName,
			&r.Status, &r.UpdatedAt); err != nil {
			return nil, st, err
		}
		out = append(out, r)
	}
	return out, st, rows.Err()
}

// IntakeOptions lists the intake/year combinations in use, e.g. "Autumn 2027".
func (s *Service) IntakeOptions(ctx context.Context) ([]model.Option, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT intake, year FROM applications
		WHERE intake IS NOT NULL AND year IS NOT NULL ORDER BY year, intake DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Option
	for rows.Next() {
		var in string
		var y int16
		if err := rows.Scan(&in, &y); err != nil {
			return nil, err
		}
		ys := strconv.Itoa(int(y))
		out = append(out, model.Option{Value: in + "-" + ys, Label: model.IntakeLabel(in) + " " + ys})
	}
	return out, rows.Err()
}
