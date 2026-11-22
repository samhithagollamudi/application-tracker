-- name: CreateExport :one
INSERT INTO exports (user_id, query, total_rows) VALUES ($1, $2, $3) RETURNING id;

-- name: FindPendingExport :one
SELECT id FROM exports WHERE user_id = $1 AND query = $2 AND status IN ('queued', 'running')
ORDER BY id DESC LIMIT 1;

-- name: ClaimExport :one
UPDATE exports SET status = 'running' WHERE id = $1 AND status = 'queued'
RETURNING id, user_id, query;

-- name: SetExportProgress :exec
UPDATE exports SET total_rows = $2, done_rows = $3 WHERE id = $1;

-- name: FinishExport :exec
UPDATE exports SET status = 'done', done_rows = $2, total_rows = $2, file = $3, file_name = $4, file_size = $5,
  finished_at = now()
WHERE id = $1;

-- name: FailExport :exec
UPDATE exports SET status = 'failed', error = $2, finished_at = now() WHERE id = $1;

-- name: RequeueInterruptedExports :exec
UPDATE exports SET status = 'queued', done_rows = 0 WHERE status = 'running';

-- name: ListQueuedExports :many
SELECT id FROM exports WHERE status = 'queued' ORDER BY id;

-- name: ListUserExports :many
SELECT id, query, status, total_rows, done_rows, file_name, file_size, error, created_at, finished_at
FROM exports WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT 20;

-- name: GetExportFile :one
SELECT file, file_name FROM exports WHERE id = $1 AND user_id = $2 AND status = 'done';

-- name: CountPendingExports :one
SELECT count(*)::int FROM exports WHERE user_id = $1 AND status IN ('queued', 'running');

-- name: TakeExportNotifications :many
UPDATE exports SET notified = true
WHERE user_id = $1 AND status IN ('done', 'failed') AND NOT notified
RETURNING id, status, total_rows;

-- name: DeleteExpiredExports :execrows
DELETE FROM exports WHERE created_at < now() - interval '24 hours' AND status IN ('done', 'failed');
