-- name: InsertActivity :exec
INSERT INTO activity_log (student_id, actor_id, action, detail) VALUES ($1, $2, $3, $4);

-- name: ListActivity :many
SELECT a.action, a.detail, a.created_at, u.name AS actor_name
FROM activity_log a JOIN users u ON u.id = a.actor_id
WHERE a.student_id = $1
ORDER BY a.created_at DESC, a.id DESC
LIMIT 15;
