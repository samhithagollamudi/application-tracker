-- name: ListUniversities :many
SELECT id, name, country FROM universities ORDER BY lower(name);

-- name: SearchUniversities :many
SELECT id, name, country FROM universities
WHERE sqlc.arg(q)::text = ''
   OR name ILIKE '%' || sqlc.arg(q)::text || '%'
   OR country ILIKE '%' || sqlc.arg(q)::text || '%'
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
