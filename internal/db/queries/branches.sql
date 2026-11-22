-- name: ListBranches :many
SELECT id, name FROM branches ORDER BY id;

-- name: GetBranch :one
SELECT id, name FROM branches WHERE id = $1;
