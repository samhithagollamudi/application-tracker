// Package catalog serves the admin-maintained lists: branches, universities
// and consultancies.
package catalog

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: gen.New(pool)} }

func (s *Service) Branches(ctx context.Context) ([]model.Branch, error) {
	rows, err := s.q.ListBranches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Branch, len(rows))
	for i, r := range rows {
		out[i] = model.Branch{ID: r.ID, Name: r.Name}
	}
	return out, nil
}

func (s *Service) BranchOptions(ctx context.Context) ([]model.Option, error) {
	bs, err := s.Branches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Option, len(bs))
	for i, b := range bs {
		out[i] = model.Option{Value: strconv.Itoa(int(b.ID)), Label: b.Name}
	}
	return out, nil
}

func (s *Service) Universities(ctx context.Context) ([]model.University, error) {
	rows, err := s.q.ListUniversities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.University, len(rows))
	for i, r := range rows {
		out[i] = model.University{ID: r.ID, Name: r.Name, Country: r.Country}
	}
	return out, nil
}

func (s *Service) UniversityOptions(ctx context.Context) ([]model.Option, error) {
	us, err := s.Universities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Option, len(us))
	for i, u := range us {
		out[i] = model.Option{Value: strconv.FormatInt(u.ID, 10), Label: u.Name}
	}
	return out, nil
}

func (s *Service) SearchUniversities(ctx context.Context, q string) ([]model.University, error) {
	rows, err := s.q.SearchUniversities(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]model.University, len(rows))
	for i, r := range rows {
		out[i] = model.University{ID: r.ID, Name: r.Name, Country: r.Country}
	}
	return out, nil
}

// University returns nil (no error) when id doesn't exist.
func University(ctx context.Context, db gen.DBTX, id int64) (*model.University, error) {
	r, err := gen.New(db).GetUniversity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.University{ID: r.ID, Name: r.Name, Country: r.Country}, nil
}

// UniversityByName matches case-insensitively; nil when absent.
func UniversityByName(ctx context.Context, db gen.DBTX, name string) (*model.University, error) {
	r, err := gen.New(db).GetUniversityByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.University{ID: r.ID, Name: r.Name, Country: r.Country}, nil
}

func (s *Service) ConsultancyOptions(ctx context.Context) ([]model.Option, error) {
	rows, err := s.q.ListConsultancies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Option, len(rows))
	for i, r := range rows {
		out[i] = model.Option{Value: strconv.FormatInt(r.ID, 10), Label: r.Name}
	}
	return out, nil
}

// ConsultancyExists reports whether id is in the list.
func ConsultancyExists(ctx context.Context, db gen.DBTX, id int64) (bool, error) {
	_, err := gen.New(db).GetConsultancy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// EnsureConsultancy returns the id of the consultancy with this name
// (case-insensitive), adding it to the list if it is new.
func EnsureConsultancy(ctx context.Context, db gen.DBTX, name string, actorID int64) (int64, error) {
	q := gen.New(db)
	r, err := q.GetConsultancyByName(ctx, name)
	if err == nil {
		return r.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	return q.CreateConsultancy(ctx, gen.CreateConsultancyParams{Name: name, CreatedBy: &actorID})
}

// Countries used anywhere (list universities + one-off applications).
func (s *Service) CountryOptions(ctx context.Context) ([]model.Option, error) {
	cs, err := s.q.ListCountries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Option, len(cs))
	for i, c := range cs {
		out[i] = model.Option{Value: c, Label: c}
	}
	return out, nil
}
