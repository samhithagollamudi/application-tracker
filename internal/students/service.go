// Package students implements the student list, detail, create, edit and
// branch move. Every query is restricted by access.StudentFilter.
package students

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/activity"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

var ErrNotFound = errors.New("student not found")

// DuplicateError means another student already uses the email.
type DuplicateError struct {
	ExistingID int64
	Visible    bool // whether the current user may open that student
}

func (e *DuplicateError) Error() string { return "student email already registered" }

// FieldErrors maps form field names to messages.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "invalid input" }

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

const studentCols = `s.id, s.source, s.branch_id, coalesce(b.name, ''), s.consultancy_id, coalesce(c.name, ''),
	s.owner_id, o.name, s.created_at, s.updated_at,
	coalesce(s.first_name, ''), coalesce(s.family_name, ''), coalesce(s.gender, ''),
	coalesce(to_char(s.date_of_birth, 'YYYY-MM-DD'), ''), s.email::text, coalesce(s.address, ''),
	coalesce(s.ba_university, ''), coalesce(s.ba_specialisation, ''), coalesce(s.ba_duration, ''), coalesce(s.ba_grade, ''),
	coalesce(s.ma_university, ''), coalesce(s.ma_specialisation, ''), coalesce(s.ma_duration, ''), coalesce(s.ma_grade, ''),
	coalesce(s.xii_board, ''), coalesce(s.xii_subjects, ''), coalesce(s.xii_grade, '')`

const studentFrom = ` FROM students s
	LEFT JOIN branches b ON b.id = s.branch_id
	LEFT JOIN consultancies c ON c.id = s.consultancy_id
	JOIN users o ON o.id = s.owner_id`

func scanStudent(row pgx.Row) (*model.Student, error) {
	var s model.Student
	f := &s.StudentFields
	err := row.Scan(&s.ID, &s.Source, &s.BranchID, &s.BranchName, &s.ConsultancyID, &s.ConsultancyName,
		&s.OwnerID, &s.OwnerName, &s.CreatedAt, &s.UpdatedAt,
		&f.FirstName, &f.FamilyName, &f.Gender, &f.DateOfBirth, &f.Email, &f.Address,
		&f.BAUniversity, &f.BASpecialisation, &f.BADuration, &f.BAGrade,
		&f.MAUniversity, &f.MASpecialisation, &f.MADuration, &f.MAGrade,
		&f.XIIBoard, &f.XIISubjects, &f.XIIGrade)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Get returns the student if visible to scope, else ErrNotFound.
func (sv *Service) Get(ctx context.Context, scope access.Scope, id int64) (*model.Student, error) {
	return get(ctx, sv.pool, scope, id, false)
}

func get(ctx context.Context, db gen.DBTX, scope access.Scope, id int64, forUpdate bool) (*model.Student, error) {
	var args access.Args
	sql := `SELECT ` + studentCols + studentFrom + ` WHERE s.id = ` + args.Add(id) +
		` AND ` + access.StudentFilter(scope, "s", &args)
	if forUpdate {
		sql += ` FOR UPDATE OF s`
	}
	s, err := scanStudent(db.QueryRow(ctx, sql, args.Values()...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

// ListFilter: empty lists mean "any"; several values match any of them.
type ListFilter struct {
	Q              string
	BranchIDs      []int16
	ConsultancyIDs []int64
	Countries      []string
	UniversityIDs  []int64
	// Application filters: a student matches when ONE of their applications
	// satisfies all of these together (each list matches any of its values).
	Statuses []string
	Levels   []string
	Intakes  []string
	Years    []int16
}

// List returns one page of matching students (most recently updated first)
// and the page with its total filled in.
func (sv *Service) List(ctx context.Context, scope access.Scope, f ListFilter, page model.Page) ([]model.StudentRow, model.Page, error) {
	var args access.Args
	where := []string{access.StudentFilter(scope, "s", &args)}
	if q := strings.TrimSpace(f.Q); q != "" {
		p := args.Add("%" + escapeLike(strings.ToLower(q)) + "%")
		cond := `(lower(coalesce(s.first_name,'') || ' ' || coalesce(s.family_name,'') || ' ' || s.email::text) LIKE ` + p +
			` OR EXISTS (SELECT 1 FROM applications a WHERE a.student_id = s.id AND lower(a.application_number) LIKE ` + p + `)`
		if id, ok := model.ParseCode(q, "STU"); ok {
			cond += ` OR s.id = ` + args.Add(id)
		}
		where = append(where, cond+`)`)
	}
	if len(f.BranchIDs) > 0 {
		where = append(where, `s.branch_id = ANY(`+args.Add(f.BranchIDs)+`)`)
	}
	if len(f.ConsultancyIDs) > 0 {
		where = append(where, `s.consultancy_id = ANY(`+args.Add(f.ConsultancyIDs)+`)`)
	}
	var app []string
	if len(f.Countries) > 0 {
		app = append(app, `coalesce(un.country, a.custom_university_country) = ANY(`+args.Add(f.Countries)+`)`)
	}
	if len(f.UniversityIDs) > 0 {
		app = append(app, `a.university_id = ANY(`+args.Add(f.UniversityIDs)+`)`)
	}
	if len(f.Statuses) > 0 {
		app = append(app, `a.status = ANY(`+args.Add(f.Statuses)+`)`)
	}
	if len(f.Levels) > 0 {
		app = append(app, `a.programme_level = ANY(`+args.Add(f.Levels)+`)`)
	}
	if len(f.Intakes) > 0 {
		app = append(app, `a.intake = ANY(`+args.Add(f.Intakes)+`)`)
	}
	if len(f.Years) > 0 {
		app = append(app, `a.year = ANY(`+args.Add(f.Years)+`)`)
	}
	if len(app) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM applications a LEFT JOIN universities un ON un.id = a.university_id
			WHERE a.student_id = s.id AND `+strings.Join(app, " AND ")+`)`)
	}

	cond := strings.Join(where, " AND ")

	var total int
	if err := sv.pool.QueryRow(ctx, `SELECT count(*) FROM students s WHERE `+cond, args.Values()...).Scan(&total); err != nil {
		return nil, page, err
	}
	page = page.WithTotal(total)
	if total == 0 {
		return nil, page, nil
	}

	// Pick the page's ids first, then compute the per-student extras for those rows only.
	lim, off := args.Add(page.Size), args.Add(page.Offset())
	sql := `WITH pg AS (
			SELECT s.id FROM students s WHERE ` + cond + `
			ORDER BY s.updated_at DESC, s.id DESC LIMIT ` + lim + ` OFFSET ` + off + `
		)
		SELECT s.id, coalesce(s.first_name,''), coalesce(s.family_name,''), s.email::text, s.source,
		coalesce(b.name,''), coalesce(c.name,''),
		(SELECT count(*) FROM applications a WHERE a.student_id = s.id),
		coalesce((SELECT string_agg(coalesce(un.name, a.custom_university_name), ', ' ORDER BY a.created_at)
			FROM applications a LEFT JOIN universities un ON un.id = a.university_id WHERE a.student_id = s.id), ''),
		(SELECT coalesce(a.status, '') FROM applications a WHERE a.student_id = s.id ORDER BY a.updated_at DESC, a.id DESC LIMIT 1),
		s.updated_at
		FROM pg JOIN students s ON s.id = pg.id
		LEFT JOIN branches b ON b.id = s.branch_id
		LEFT JOIN consultancies c ON c.id = s.consultancy_id
		ORDER BY s.updated_at DESC, s.id DESC`

	rows, err := sv.pool.Query(ctx, sql, args.Values()...)
	if err != nil {
		return nil, page, err
	}
	defer rows.Close()
	var out []model.StudentRow
	for rows.Next() {
		var r model.StudentRow
		if err := rows.Scan(&r.ID, &r.FirstName, &r.FamilyName, &r.Email, &r.Source, &r.BranchName,
			&r.ConsultancyName, &r.AppCount, &r.Universities, &r.LatestStatus, &r.UpdatedAt); err != nil {
			return nil, page, err
		}
		out = append(out, r)
	}
	return out, page, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Validate normalises f in place and reports field errors. Only the email is required.
func Validate(f *model.StudentFields) FieldErrors {
	errs := FieldErrors{}
	for _, p := range []*string{&f.FirstName, &f.FamilyName, &f.Gender, &f.DateOfBirth, &f.Email, &f.Address,
		&f.BAUniversity, &f.BASpecialisation, &f.BADuration, &f.BAGrade,
		&f.MAUniversity, &f.MASpecialisation, &f.MADuration, &f.MAGrade,
		&f.XIIBoard, &f.XIISubjects, &f.XIIGrade} {
		*p = strings.TrimSpace(*p)
	}

	switch {
	case f.Email == "":
		errs["email"] = "The application email is required."
	case len(f.Email) > 254:
		errs["email"] = "That email is too long."
	default:
		if a, err := mail.ParseAddress(f.Email); err != nil || a.Address != f.Email || !strings.Contains(a.Address[strings.LastIndex(a.Address, "@"):], ".") {
			errs["email"] = "Enter a valid email address, like name@example.com."
		}
	}
	if f.Gender != "" && !model.ValidOption(model.Genders, f.Gender) {
		errs["gender"] = "Pick a gender from the list."
	}
	if f.DateOfBirth != "" {
		d, err := time.Parse("2006-01-02", f.DateOfBirth)
		if err != nil || d.Year() < 1900 || d.After(time.Now()) {
			errs["date_of_birth"] = "Enter a real date of birth."
		}
	}
	limits := map[string]struct {
		v   string
		max int
	}{
		"first_name": {f.FirstName, 100}, "family_name": {f.FamilyName, 100}, "address": {f.Address, 1000},
		"ba_university": {f.BAUniversity, 200}, "ba_specialisation": {f.BASpecialisation, 200}, "ba_duration": {f.BADuration, 50}, "ba_grade": {f.BAGrade, 50},
		"ma_university": {f.MAUniversity, 200}, "ma_specialisation": {f.MASpecialisation, 200}, "ma_duration": {f.MADuration, 50}, "ma_grade": {f.MAGrade, 50},
		"xii_board": {f.XIIBoard, 200}, "xii_subjects": {f.XIISubjects, 200}, "xii_grade": {f.XIIGrade, 50},
	}
	for name, l := range limits {
		if utf8.RuneCountInString(l.v) > l.max {
			errs[name] = fmt.Sprintf("Keep this under %d characters.", l.max)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// checkDuplicate returns a *DuplicateError if another student has this email.
func checkDuplicate(ctx context.Context, db gen.DBTX, scope access.Scope, email string, excludeID int64) error {
	var id int64
	var source string
	var branchID *int16
	err := db.QueryRow(ctx, `SELECT id, source, branch_id FROM students
		WHERE lower(btrim(email::text)) = lower(btrim($1)) AND id <> $2`, email, excludeID).Scan(&id, &source, &branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return &DuplicateError{ExistingID: id, Visible: scope.CanSee(source, branchID)}
}

func isEmailUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "students_email_uq"
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fieldArgs(f *model.StudentFields) []any {
	return []any{nullable(f.FirstName), nullable(f.FamilyName), nullable(f.Gender), nullable(f.DateOfBirth), f.Email, nullable(f.Address),
		nullable(f.BAUniversity), nullable(f.BASpecialisation), nullable(f.BADuration), nullable(f.BAGrade),
		nullable(f.MAUniversity), nullable(f.MASpecialisation), nullable(f.MADuration), nullable(f.MAGrade),
		nullable(f.XIIBoard), nullable(f.XIISubjects), nullable(f.XIIGrade)}
}

// AfterCreate runs inside the create transaction (used to add the first application).
type AfterCreate func(ctx context.Context, tx pgx.Tx, studentID int64) error

// Create adds a student owned by actor. Consultants create internal students
// in their branch; the agent network creates B2B students.
func (sv *Service) Create(ctx context.Context, actor *model.User, f model.StudentFields, consultancy ConsultancyChoice, after AfterCreate) (int64, error) {
	scope := access.ScopeOf(actor)
	if !scope.CanCreateStudents() {
		return 0, errors.New("role cannot create students")
	}
	if errs := Validate(&f); errs != nil {
		return 0, errs
	}
	source, branchID := scope.NewStudentSource()

	var id int64
	err := pgx.BeginFunc(ctx, sv.pool, func(tx pgx.Tx) error {
		if err := checkDuplicate(ctx, tx, scope, f.Email, 0); err != nil {
			return err
		}
		var consID *int64
		if source == "b2b" {
			var err error
			if consID, err = consultancy.resolve(ctx, tx, actor.ID); err != nil {
				return err
			}
		}
		args := append([]any{source, branchID, consID, actor.ID}, fieldArgs(&f)...)
		if err := tx.QueryRow(ctx, `INSERT INTO students (source, branch_id, consultancy_id, owner_id,
			first_name, family_name, gender, date_of_birth, email, address,
			ba_university, ba_specialisation, ba_duration, ba_grade,
			ma_university, ma_specialisation, ma_duration, ma_grade,
			xii_board, xii_subjects, xii_grade)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
			RETURNING id`, args...).Scan(&id); err != nil {
			return err
		}
		if err := activity.Log(ctx, tx, id, actor.ID, activity.StudentCreated, "Student created"); err != nil {
			return err
		}
		if after != nil {
			return after(ctx, tx, id)
		}
		return nil
	})
	if isEmailUniqueViolation(err) {
		// Lost a race with another insert; report it like any duplicate.
		if derr := checkDuplicate(ctx, sv.pool, scope, f.Email, 0); derr != nil {
			return 0, derr
		}
	}
	return id, err
}

// Update edits a visible student's details. consultancy is applied only for
// B2B students and only when the editor may change it (not consultants).
func (sv *Service) Update(ctx context.Context, actor *model.User, id int64, f model.StudentFields, consultancy *ConsultancyChoice) error {
	scope := access.ScopeOf(actor)
	if errs := Validate(&f); errs != nil {
		return errs
	}
	err := pgx.BeginFunc(ctx, sv.pool, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, scope, id, true)
		if err != nil {
			return err
		}
		if err := checkDuplicate(ctx, tx, scope, f.Email, id); err != nil {
			return err
		}
		consID := cur.ConsultancyID
		if consultancy != nil && cur.Source == "b2b" && actor.Role != model.RoleConsultant {
			if consID, err = consultancy.resolve(ctx, tx, actor.ID); err != nil {
				return err
			}
		}
		args := append([]any{id, consID}, fieldArgs(&f)...)
		if _, err := tx.Exec(ctx, `UPDATE students SET consultancy_id = $2,
			first_name = $3, family_name = $4, gender = $5, date_of_birth = $6, email = $7, address = $8,
			ba_university = $9, ba_specialisation = $10, ba_duration = $11, ba_grade = $12,
			ma_university = $13, ma_specialisation = $14, ma_duration = $15, ma_grade = $16,
			xii_board = $17, xii_subjects = $18, xii_grade = $19, updated_at = now()
			WHERE id = $1`, args...); err != nil {
			return err
		}
		return activity.Log(ctx, tx, id, actor.ID, activity.StudentUpdated, "Student details updated")
	})
	if isEmailUniqueViolation(err) {
		if derr := checkDuplicate(ctx, sv.pool, scope, f.Email, id); derr != nil {
			return derr
		}
	}
	return err
}

// Move puts an internal student in another branch (admin only). The owner
// becomes that branch's active consultant when there is one.
func (sv *Service) Move(ctx context.Context, actor *model.User, id int64, branchID int16) (string, error) {
	if !actor.IsAdmin() {
		return "", ErrNotFound
	}
	var msg string
	err := pgx.BeginFunc(ctx, sv.pool, func(tx pgx.Tx) error {
		s, err := get(ctx, tx, access.ScopeOf(actor), id, true)
		if err != nil {
			return err
		}
		if s.Source != "internal" {
			msg = "B2B students don’t belong to a branch, so they can’t be moved."
			return nil
		}
		q := gen.New(tx)
		b, err := q.GetBranch(ctx, branchID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if s.BranchID != nil && *s.BranchID == branchID {
			msg = "Already in " + b.Name
			return nil
		}
		owner := s.OwnerID
		if cid, err := q.ActiveConsultantForBranch(ctx, &branchID); err == nil {
			owner = cid
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE students SET branch_id = $2, owner_id = $3, updated_at = now() WHERE id = $1`,
			id, branchID, owner); err != nil {
			return err
		}
		msg = "Moved to " + b.Name
		return activity.Log(ctx, tx, id, actor.ID, activity.StudentMoved, "Moved from "+s.BranchName+" to "+b.Name)
	})
	return msg, err
}
