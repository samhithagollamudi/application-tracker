-- name: ListUniversities :many
SELECT id, name, country FROM universities ORDER BY lower(name);

-- name: SearchUniversities :many
SELECT id, name, country FROM universities
WHERE (sqlc.arg(country)::text = '' OR country = sqlc.arg(country)::text)
  AND (sqlc.arg(q)::text = ''
   OR name ILIKE '%' || sqlc.arg(q)::text || '%'
   OR country ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY lower(name)
LIMIT 30;

-- name: GetUniversity :one
SELECT id, name, country FROM universities WHERE id = $1;

-- name: GetUniversityByName :one
SELECT id, name, country FROM universities WHERE lower(name) = lower(sqlc.arg(name)::text);

-- name: ListConsultancies :many
SELECT id, name FROM consultancies ORDER BY lower(name);

-- name: GetConsultancy :one
SELECT id, name FROM consultancies WHERE id = $1;

-- name: GetConsultancyByName :one
SELECT id, name FROM consultancies WHERE lower(name) = lower(sqlc.arg(name)::text);

-- name: CreateConsultancy :one
INSERT INTO consultancies (name, created_by) VALUES ($1, $2) RETURNING id;

-- name: ListCountries :many
SELECT country::text AS country FROM universities
UNION
SELECT custom_university_country::text FROM applications WHERE custom_university_country IS NOT NULL
ORDER BY 1;

-- name: ListUniversitiesAdmin :many
SELECT u.id, u.name, u.country, cb.name AS added_by_name, cb.role AS added_by_role,
       (SELECT count(*) FROM applications a WHERE a.university_id = u.id)::int AS uses
FROM universities u LEFT JOIN users cb ON cb.id = u.created_by
ORDER BY lower(u.name);

-- name: ListConsultanciesAdmin :many
SELECT c.id, c.name, cb.name AS added_by_name, cb.role AS added_by_role,
       (SELECT count(*) FROM students s WHERE s.consultancy_id = c.id)::int AS uses
FROM consultancies c LEFT JOIN users cb ON cb.id = c.created_by
ORDER BY lower(c.name);

-- name: CreateUniversity :one
INSERT INTO universities (name, country, created_by) VALUES ($1, $2, $3) RETURNING id;

-- name: DeleteUnusedUniversity :execrows
DELETE FROM universities u WHERE u.id = $1
  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.university_id = u.id);

-- name: DeleteUnusedConsultancy :execrows
DELETE FROM consultancies c WHERE c.id = $1
  AND NOT EXISTS (SELECT 1 FROM students s WHERE s.consultancy_id = c.id);

-- name: ListCountryNames :many
SELECT name FROM countries ORDER BY lower(name);

-- name: GetCountryName :one
SELECT name FROM countries WHERE lower(name) = lower(sqlc.arg(name)::text);

-- name: ListCountriesAdmin :many
SELECT c.id, c.name,
       (SELECT count(*) FROM universities u WHERE u.country = c.name)::int AS universities,
       (SELECT count(*) FROM applications a WHERE a.custom_university_country = c.name)::int AS one_off_applications
FROM countries c ORDER BY lower(c.name);

-- name: CreateCountry :one
INSERT INTO countries (name) VALUES ($1) RETURNING id;

-- name: DeleteUnusedCountry :execrows
DELETE FROM countries c WHERE c.id = $1
  AND NOT EXISTS (SELECT 1 FROM universities u WHERE u.country = c.name)
  AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.custom_university_country = c.name);

-- name: ListUsedYears :many
SELECT DISTINCT year FROM applications WHERE year IS NOT NULL ORDER BY year;
