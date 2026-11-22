package applications

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/activity"
	"github.com/ChandraShekar2001/application-tracker/internal/catalog"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Input is a validated application ready to store.
type Input struct {
	UniversityID   *int64
	CustomName     *string
	CustomCountry  *string
	Level          *string
	Programme      *string
	Priority       *int32
	Intake         *string
	Year           *int16
	AppNumber      *string
	SubmissionDate *string
	Status         *string
	PersonalCode   *string
	TuitionFee     *string
	Currency       *string
	Per            *string
	Scholarship    *string

	UniLabel string // university name, for activity text
	Country  string
}

// ParseFields reads the application form fields (all strings, trimmed).
func ParseFields(r *http.Request) model.ApplicationFields {
	v := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	return model.ApplicationFields{
		UniversityID: v("university_id"), UniversityName: v("university_name"),
		UniOther: strings.Join(strings.Fields(v("uni_other")), " "), UniOtherCountry: v("uni_other_country"),
		Level: v("programme_level"), Programme: v("programme"), Priority: v("priority"),
		Intake: v("intake"), Year: v("year"), ApplicationNumber: v("application_number"),
		SubmissionDate: v("submission_date"), Status: v("status"), PersonalCode: v("personal_code"),
		TuitionFee: v("tuition_fee"), Currency: v("tuition_currency"), TuitionPer: v("tuition_per"),
		Scholarship: v("scholarship"),
	}
}

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Resolve validates f and resolves the university. A one-off name that
// matches a list university (case-insensitive) uses the list entry; other
// one-off names stay on this application only and are never added to the
// list. errs is non-nil when the user must fix something.
func (s *Service) Resolve(ctx context.Context, f *model.ApplicationFields) (*Input, map[string]string, error) {
	errs := map[string]string{}
	in := &Input{}

	switch {
	case f.UniversityID != "":
		id, err := strconv.ParseInt(f.UniversityID, 10, 64)
		var u *model.University
		if err == nil {
			if u, err = catalog.University(ctx, s.pool, id); err != nil {
				return nil, nil, err
			}
		}
		if u == nil {
			errs["university"] = "That university is not in the list any more. Search again."
			f.UniversityID, f.UniversityName = "", ""
			break
		}
		in.UniversityID, in.UniLabel, in.Country = &u.ID, u.Name, u.Country
		f.UniversityName, f.UniOther, f.UniOtherCountry = u.Name, "", ""
	case f.UniOther != "":
		if utf8.RuneCountInString(f.UniOther) > 200 {
			errs["university"] = "Keep the university name under 200 characters."
			break
		}
		u, err := catalog.UniversityByName(ctx, s.pool, f.UniOther)
		if err != nil {
			return nil, nil, err
		}
		if u != nil {
			in.UniversityID, in.UniLabel, in.Country = &u.ID, u.Name, u.Country
			f.UniversityID, f.UniversityName, f.UniOther, f.UniOtherCountry = strconv.FormatInt(u.ID, 10), u.Name, "", ""
			break
		}
		if f.UniOtherCountry != "" && !contains(model.Countries, f.UniOtherCountry) {
			errs["uni_other_country"] = "Pick a country from the list."
		}
		in.CustomName, in.CustomCountry = &f.UniOther, opt(f.UniOtherCountry)
		in.UniLabel, in.Country = f.UniOther, f.UniOtherCountry
	default:
		errs["university"] = "Pick a university, or choose “Other” and type one."
	}

	check := func(field, v string, opts []model.Option, msg string) *string {
		if v == "" {
			return nil
		}
		if !model.ValidOption(opts, v) {
			errs[field] = msg
			return nil
		}
		return &v
	}
	in.Level = check("programme_level", f.Level, model.Levels, "Pick a level from the list.")
	in.Intake = check("intake", f.Intake, model.Intakes, "Pick an intake from the list.")
	in.Status = check("status", f.Status, model.Statuses, "Pick a status from the list.")
	in.Per = check("tuition_per", f.TuitionPer, model.TuitionPer, "Pick from the list.")

	text := func(field, v string, max int) *string {
		if utf8.RuneCountInString(v) > max {
			errs[field] = fmt.Sprintf("Keep this under %d characters.", max)
		}
		return opt(v)
	}
	in.Programme = text("programme", f.Programme, 200)
	in.AppNumber = text("application_number", f.ApplicationNumber, 100)
	in.PersonalCode = text("personal_code", f.PersonalCode, 50)
	in.Scholarship = text("scholarship", f.Scholarship, 100)

	if f.Priority != "" {
		n, err := strconv.ParseInt(f.Priority, 10, 32)
		if err != nil || n < 0 || n > 999 {
			errs["priority"] = "Use a whole number (0–999)."
		} else {
			p := int32(n)
			in.Priority = &p
		}
	}
	if f.Year != "" {
		n, err := strconv.Atoi(f.Year)
		if err != nil || n < 2000 || n > 2100 {
			errs["year"] = "Pick a year from the list."
		} else {
			y := int16(n)
			in.Year = &y
		}
	}
	if f.SubmissionDate != "" {
		if d, err := time.Parse("2006-01-02", f.SubmissionDate); err != nil || d.Year() < 2000 || d.Year() > 2100 {
			errs["submission_date"] = "Enter a valid date."
		} else {
			in.SubmissionDate = &f.SubmissionDate
		}
	}
	if f.TuitionFee != "" {
		fee := strings.ReplaceAll(strings.ReplaceAll(f.TuitionFee, ",", ""), " ", "")
		v, err := strconv.ParseFloat(fee, 64)
		dot := strings.IndexByte(fee, '.')
		if err != nil || v < 0 || v >= 1e10 || (dot >= 0 && len(fee)-dot-1 > 2) || strings.ContainsAny(fee, "eE+-") {
			errs["tuition_fee"] = "Enter an amount like 120000 or 9500.50."
		} else {
			in.TuitionFee = &fee
			cur := f.Currency
			if cur == "" {
				cur = "EUR"
			}
			if !contains(model.Currencies, cur) {
				errs["tuition_fee"] = "Pick a currency from the list."
			}
			in.Currency = &cur
		}
	}
	if len(errs) > 0 {
		return in, errs, nil
	}
	return in, nil, nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// YearOptions: last year to four years ahead, plus current if outside that.
func YearOptions(current string) []model.Option {
	now := time.Now().Year()
	var out []model.Option
	seen := false
	for y := now - 1; y <= now+4; y++ {
		s := strconv.Itoa(y)
		seen = seen || s == current
		out = append(out, model.Option{Value: s, Label: s})
	}
	if current != "" && !seen {
		out = append([]model.Option{{Value: current, Label: current}}, out...)
	}
	return out
}

// FormVM builds the application form view model.
func (s *Service) FormVM(f model.ApplicationFields, errs map[string]string, country string) pages.AppFormVM {
	if errs == nil {
		errs = map[string]string{}
	}
	return pages.AppFormVM{F: f, Errs: errs, Country: country, Years: YearOptions(f.Year)}
}

func (in *Input) args() []any {
	return []any{in.UniversityID, in.CustomName, in.CustomCountry, in.Level, in.Programme, in.Priority,
		in.Intake, in.Year, in.AppNumber, in.SubmissionDate, in.Status, in.PersonalCode,
		in.TuitionFee, in.Currency, in.Per, in.Scholarship}
}

func touch(ctx context.Context, db gen.DBTX, studentID int64) error {
	_, err := db.Exec(ctx, `UPDATE students SET updated_at = now() WHERE id = $1`, studentID)
	return err
}

func statusText(in *Input) string {
	if in.Status == nil {
		return ""
	}
	return " (" + model.StatusLabel(*in.Status) + ")"
}

// Insert adds an application inside tx. The caller has verified the student is in scope.
func (s *Service) Insert(ctx context.Context, tx pgx.Tx, actorID, studentID int64, in *Input) (int64, error) {
	var id int64
	args := append([]any{studentID, actorID}, in.args()...)
	err := tx.QueryRow(ctx, `INSERT INTO applications (student_id, created_by,
		university_id, custom_university_name, custom_university_country, programme_level, programme, priority,
		intake, year, application_number, submission_date, status, personal_code,
		tuition_fee, tuition_currency, tuition_per, scholarship)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id`, args...).Scan(&id)
	if err != nil {
		return 0, err
	}
	if err := activity.Log(ctx, tx, studentID, actorID, activity.ApplicationAdded, "Application added: "+in.UniLabel+statusText(in)); err != nil {
		return 0, err
	}
	return id, touch(ctx, tx, studentID)
}

// studentInScope locks and returns a visible student's id and display name.
func studentInScope(ctx context.Context, db gen.DBTX, scope access.Scope, id int64) (string, error) {
	var args access.Args
	var first, family string
	err := db.QueryRow(ctx, `SELECT coalesce(s.first_name,''), coalesce(s.family_name,'') FROM students s
		WHERE s.id = `+args.Add(id)+` AND `+access.StudentFilter(scope, "s", &args)+` FOR UPDATE OF s`,
		args.Values()...).Scan(&first, &family)
	if err == pgx.ErrNoRows {
		return "", ErrNotFound
	}
	return model.FullName(first, family), err
}

// StudentName returns a visible student's display name, or ErrNotFound.
func (s *Service) StudentName(ctx context.Context, scope access.Scope, studentID int64) (string, error) {
	var name string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		name, err = studentInScope(ctx, tx, scope, studentID)
		return err
	})
	return name, err
}

// Add creates an application for a visible student.
func (s *Service) Add(ctx context.Context, actor *model.User, studentID int64, in *Input) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := studentInScope(ctx, tx, access.ScopeOf(actor), studentID); err != nil {
			return err
		}
		var err error
		id, err = s.Insert(ctx, tx, actor.ID, studentID, in)
		return err
	})
	return id, err
}

// Get returns an application whose student is visible.
func (s *Service) Get(ctx context.Context, scope access.Scope, id int64) (*model.Application, error) {
	return getScoped(ctx, s.pool, scope, id, false)
}

// Update replaces an application's fields.
func (s *Service) Update(ctx context.Context, actor *model.User, id int64, in *Input) (studentID int64, err error) {
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := getScoped(ctx, tx, access.ScopeOf(actor), id, true)
		if err != nil {
			return err
		}
		studentID = cur.StudentID
		args := append([]any{id}, in.args()...)
		if _, err := tx.Exec(ctx, `UPDATE applications SET
			university_id = $2, custom_university_name = $3, custom_university_country = $4,
			programme_level = $5, programme = $6, priority = $7, intake = $8, year = $9,
			application_number = $10, submission_date = $11, status = $12, personal_code = $13,
			tuition_fee = $14, tuition_currency = $15, tuition_per = $16, scholarship = $17,
			updated_at = now()
			WHERE id = $1`, args...); err != nil {
			return err
		}
		text := "Application edited: " + in.UniLabel
		newStatus := ""
		if in.Status != nil {
			newStatus = *in.Status
		}
		if newStatus != cur.Status {
			text += " · status " + dashLabel(cur.Status) + " → " + dashLabel(newStatus)
		}
		if err := activity.Log(ctx, tx, studentID, actor.ID, activity.ApplicationEdited, text); err != nil {
			return err
		}
		return touch(ctx, tx, studentID)
	})
	return studentID, err
}

func dashLabel(status string) string {
	if status == "" {
		return "—"
	}
	return model.StatusLabel(status)
}

// SetStatus changes only the status ("" clears it) and returns the updated application.
func (s *Service) SetStatus(ctx context.Context, actor *model.User, id int64, status string) (*model.Application, error) {
	if status != "" && !model.ValidOption(model.Statuses, status) {
		return nil, fmt.Errorf("invalid status %q", status)
	}
	var out *model.Application
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		scope := access.ScopeOf(actor)
		cur, err := getScoped(ctx, tx, scope, id, true)
		if err != nil {
			return err
		}
		if cur.Status != status {
			if _, err := tx.Exec(ctx, `UPDATE applications SET status = $2, updated_at = now() WHERE id = $1`, id, opt(status)); err != nil {
				return err
			}
			text := "Status " + dashLabel(cur.Status) + " → " + dashLabel(status) + " · " + cur.Uni
			if err := activity.Log(ctx, tx, cur.StudentID, actor.ID, activity.ApplicationStatus, text); err != nil {
				return err
			}
			if err := touch(ctx, tx, cur.StudentID); err != nil {
				return err
			}
		}
		out, err = getScoped(ctx, tx, scope, id, false)
		return err
	})
	return out, err
}

// Delete removes an application and returns its student id.
func (s *Service) Delete(ctx context.Context, actor *model.User, id int64) (studentID int64, err error) {
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := getScoped(ctx, tx, access.ScopeOf(actor), id, true)
		if err != nil {
			return err
		}
		studentID = cur.StudentID
		if _, err := tx.Exec(ctx, `DELETE FROM applications WHERE id = $1`, id); err != nil {
			return err
		}
		if err := activity.Log(ctx, tx, studentID, actor.ID, activity.ApplicationDeleted, "Application deleted: "+cur.Uni+" ("+cur.Code()+")"); err != nil {
			return err
		}
		return touch(ctx, tx, studentID)
	})
	return studentID, err
}
