-- auth_version is copied into the session at login. Bumping it (password
-- reset, deactivation) invalidates every existing session for that user.

-- +goose Up
ALTER TABLE users ADD COLUMN auth_version integer NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE users DROP COLUMN auth_version;
