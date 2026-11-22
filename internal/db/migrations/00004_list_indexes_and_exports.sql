-- Indexes matching the list sort order (most recently updated first), so
-- paging stays fast at 1 lakh+ students, plus background CSV export jobs.

-- +goose Up
CREATE INDEX students_updated_idx ON students (updated_at DESC, id DESC);
CREATE INDEX applications_updated_idx ON applications (updated_at DESC, id DESC);

CREATE TABLE exports (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  query text NOT NULL,                   -- the admin filter query string the export was made from
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
  total_rows integer NOT NULL DEFAULT 0,
  done_rows integer NOT NULL DEFAULT 0,
  file bytea,                            -- gzip-compressed CSV, deleted with the row after 24 hours
  file_name text,
  file_size bigint,                      -- uncompressed bytes
  error text,
  notified boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);
CREATE INDEX exports_user_idx ON exports (user_id, created_at DESC);
CREATE INDEX exports_pending_idx ON exports (status) WHERE status IN ('queued', 'running');

-- +goose Down
DROP TABLE exports;
DROP INDEX applications_updated_idx;
DROP INDEX students_updated_idx;
