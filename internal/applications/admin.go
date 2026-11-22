package applications

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// Filter is the admin applications filter set, parsed from the URL so views
// are bookmarkable and the CSV export uses exactly the same rows. Every list
// filter accepts several values (repeated query params); an empty list means "any".
type Filter struct {
	Q              string
	BranchIDs      []int16
	Countries      []string
	UniversityIDs  []int64
	Statuses       []string
	Intakes        []string // "autumn-2027"
	Levels         []string
	Sources        []string // internal | b2b
	ConsultancyIDs []int64
	NoConsultancy  bool   // "none" was ticked: include students without a consultancy
	From, To       string // YYYY-MM-DD, created date (IST), inclusive
}

var sources = []model.Option{{Value: "internal"}, {Value: "b2b"}}

func validIntakes(vals []string) []string {
	var out []string
	for _, v := range model.NonEmpty(vals) {
		in, y, ok := strings.Cut(v, "-")
		if _, err := strconv.ParseInt(y, 10, 16); ok && err == nil && model.ValidOption(model.Intakes, in) {
			out = append(out, v)
		}
	}
	return out
}

// ParseFilter reads a Filter from query values, ignoring invalid entries.
func ParseFilter(q url.Values) Filter {
	f := Filter{
		Q:             strings.TrimSpace(q.Get("q")),
		BranchIDs:     model.Int16s(q["branch"]),
		Countries:     model.NonEmpty(q["country"]),
		UniversityIDs: model.Int64s(q["university"]),
		Statuses:      model.ValidValues(model.Statuses, q["status"]),
		Intakes:       validIntakes(q["intake"]),
		Levels:        model.ValidValues(model.Levels, q["level"]),
		Sources:       model.ValidValues(sources, q["source"]),
	}
	f.ConsultancyIDs = model.Int64s(q["consultancy"])
	f.NoConsultancy = model.In("none", q["consultancy"])
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
	if len(f.BranchIDs) > 0 {
		w = append(w, `s.branch_id = ANY(`+args.Add(f.BranchIDs)+`)`)
	}
	if len(f.Countries) > 0 {
		w = append(w, `coalesce(u.country, a.custom_university_country) = ANY(`+args.Add(f.Countries)+`)`)
	}
	if len(f.UniversityIDs) > 0 {
		w = append(w, `a.university_id = ANY(`+args.Add(f.UniversityIDs)+`)`)
	}
	if len(f.Statuses) > 0 {
		w = append(w, `a.status = ANY(`+args.Add(f.Statuses)+`)`)
	}
	if len(f.Intakes) > 0 {
		w = append(w, `(a.intake || '-' || a.year::text) = ANY(`+args.Add(f.Intakes)+`)`)
	}
	if len(f.Levels) > 0 {
		w = append(w, `a.programme_level = ANY(`+args.Add(f.Levels)+`)`)
	}
	if len(f.Sources) > 0 {
		w = append(w, `s.source = ANY(`+args.Add(f.Sources)+`)`)
	}
	switch {
	case f.NoConsultancy && len(f.ConsultancyIDs) > 0:
		w = append(w, `(s.consultancy_id IS NULL OR s.consultancy_id = ANY(`+args.Add(f.ConsultancyIDs)+`))`)
	case f.NoConsultancy:
		w = append(w, `s.consultancy_id IS NULL`)
	case len(f.ConsultancyIDs) > 0:
		w = append(w, `s.consultancy_id = ANY(`+args.Add(f.ConsultancyIDs)+`)`)
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

// Search returns one page of rows (most recently updated first), stats over
// all matches, and the page with its total filled in.
func (s *Service) Search(ctx context.Context, scope access.Scope, f Filter, page model.Page) ([]model.AppRow, Stats, model.Page, error) {
	var st Stats
	var args access.Args
	w := where(scope, f, &args)
	err := s.pool.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE a.status IN ('applied', 'reserve')),
		count(*) FILTER (WHERE a.status IN ('admitted', 'fees_paid', 'visa_granted', 'visa_rejected')),
		count(*) FILTER (WHERE s.source = 'b2b')`+fromSQL+` WHERE `+w, args.Values()...).
		Scan(&st.Total, &st.Waiting, &st.Admitted, &st.B2B)
	if err != nil {
		return nil, st, page, err
	}
	page = page.WithTotal(st.Total)
	if st.Total == 0 {
		return nil, st, page, nil
	}

	lim, off := args.Add(page.Size), args.Add(page.Offset())
	rows, err := s.pool.Query(ctx, `SELECT a.id, s.id, coalesce(s.first_name,''), coalesce(s.family_name,''), s.email::text,
		coalesce(u.name, a.custom_university_name, ''), coalesce(u.country, a.custom_university_country, ''), a.university_id IS NULL,
		coalesce(a.programme, ''), coalesce(a.programme_level, ''), coalesce(a.intake, ''), coalesce(a.year::text, ''),
		coalesce(b.name, ''), coalesce(c.name, ''), o.name, coalesce(a.status, ''), a.updated_at`+fromSQL+
		` WHERE `+w+` ORDER BY a.updated_at DESC, a.id DESC LIMIT `+lim+` OFFSET `+off, args.Values()...)
	if err != nil {
		return nil, st, page, err
	}
	defer rows.Close()
	var out []model.AppRow
	for rows.Next() {
		var r model.AppRow
		if err := rows.Scan(&r.ID, &r.StudentID, &r.FirstName, &r.FamilyName, &r.Email, &r.Uni, &r.Country, &r.Custom,
			&r.Programme, &r.Level, &r.Intake, &r.Year, &r.BranchName, &r.ConsultancyName, &r.OwnerName,
			&r.Status, &r.UpdatedAt); err != nil {
			return nil, st, page, err
		}
		out = append(out, r)
	}
	return out, st, page, rows.Err()
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

// Count returns how many applications match f (used to decide whether an
// export can stream straight away or must run in the background).
func Count(ctx context.Context, db gen.DBTX, scope access.Scope, f Filter) (int, error) {
	var args access.Args
	var n int
	err := db.QueryRow(ctx, `SELECT count(*)`+fromSQL+` WHERE `+where(scope, f, &args), args.Values()...).Scan(&n)
	return n, err
}
