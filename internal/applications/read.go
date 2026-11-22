// Package applications manages a student's university applications.
// Visibility always comes from the parent student (see access.StudentFilter).
package applications

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

var ErrNotFound = errors.New("application not found")

const appCols = `a.id, a.student_id, a.university_id,
	coalesce(u.name, a.custom_university_name, ''), coalesce(u.country, a.custom_university_country, ''),
	a.university_id IS NULL,
	coalesce(a.custom_university_name, ''), coalesce(a.custom_university_country, ''),
	coalesce(a.programme_level, ''), coalesce(a.programme, ''), coalesce(a.priority::text, ''),
	coalesce(a.intake, ''), coalesce(a.year::text, ''), coalesce(a.application_number, ''),
	coalesce(to_char(a.submission_date, 'YYYY-MM-DD'), ''), coalesce(a.status, ''),
	coalesce(a.personal_code, ''), coalesce(a.tuition_fee::text, ''), coalesce(a.tuition_currency, ''),
	coalesce(a.tuition_per, ''), coalesce(a.scholarship, ''), a.created_at, a.updated_at`

const appFrom = ` FROM applications a LEFT JOIN universities u ON u.id = a.university_id`

func scanApp(row pgx.Row) (*model.Application, error) {
	var a model.Application
	var uniID *int64
	err := row.Scan(&a.ID, &a.StudentID, &uniID, &a.Uni, &a.Country, &a.Custom,
		&a.UniOther, &a.UniOtherCountry, &a.Level, &a.Programme, &a.Priority,
		&a.Intake, &a.Year, &a.ApplicationNumber, &a.SubmissionDate, &a.Status,
		&a.PersonalCode, &a.TuitionFee, &a.Currency, &a.TuitionPer, &a.Scholarship,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if uniID != nil {
		a.UniversityID = strconv.FormatInt(*uniID, 10)
		a.UniversityName = a.Uni
	}
	return &a, nil
}

// ListForStudent returns a student's applications, newest first. The caller
// must already have loaded the student through a scoped query.
func ListForStudent(ctx context.Context, db gen.DBTX, studentID int64) ([]model.Application, error) {
	rows, err := db.Query(ctx, `SELECT `+appCols+appFrom+` WHERE a.student_id = $1 ORDER BY a.created_at DESC, a.id DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// getScoped loads one application if its student is visible to scope.
// forUpdate locks the application row.
func getScoped(ctx context.Context, db gen.DBTX, scope access.Scope, id int64, forUpdate bool) (*model.Application, error) {
	var args access.Args
	sql := `SELECT ` + appCols + appFrom + ` JOIN students s ON s.id = a.student_id
		WHERE a.id = ` + args.Add(id) + ` AND ` + access.StudentFilter(scope, "s", &args)
	if forUpdate {
		sql += ` FOR UPDATE OF a`
	}
	a, err := scanApp(db.QueryRow(ctx, sql, args.Values()...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}
