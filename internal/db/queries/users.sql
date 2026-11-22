-- name: GetUserByID :one
SELECT u.id, u.name, u.email, u.role, u.branch_id, b.name AS branch_name,
       u.password_hash, u.active, u.auth_version, u.created_at
FROM users u LEFT JOIN branches b ON b.id = u.branch_id
WHERE u.id = $1;

-- name: GetUserByEmail :one
SELECT u.id, u.name, u.email, u.role, u.branch_id, b.name AS branch_name,
       u.password_hash, u.active, u.auth_version, u.created_at
FROM users u LEFT JOIN branches b ON b.id = u.branch_id
WHERE u.email = $1;

-- name: ListUsers :many
SELECT u.id, u.name, u.email, u.role, u.branch_id, b.name AS branch_name,
       u.active, u.created_at
FROM users u LEFT JOIN branches b ON b.id = u.branch_id
ORDER BY CASE u.role WHEN 'admin' THEN 0 WHEN 'consultant' THEN 1 ELSE 2 END, b.id NULLS LAST, u.name;

-- name: CreateUser :one
INSERT INTO users (name, email, role, branch_id, password_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = $2, auth_version = auth_version + 1 WHERE id = $1;

-- name: SetUserActive :exec
UPDATE users SET active = $2, auth_version = auth_version + 1 WHERE id = $1;

-- name: ActiveConsultantForBranch :one
SELECT id FROM users WHERE role = 'consultant' AND active AND branch_id = $1;

-- name: FreeConsultantBranches :many
SELECT b.id, b.name FROM branches b
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.branch_id = b.id AND u.role = 'consultant' AND u.active)
ORDER BY b.id;

-- name: DatabaseSize :one
SELECT pg_database_size(current_database())::bigint;
