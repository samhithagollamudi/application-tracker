-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE branches (
  id smallserial PRIMARY KEY,
  name text NOT NULL UNIQUE
);

CREATE TABLE users (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  email citext NOT NULL UNIQUE,
  role text NOT NULL CHECK (role IN ('admin','consultant','agent_network')),
  branch_id smallint REFERENCES branches(id),
  password_hash text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((role = 'consultant') = (branch_id IS NOT NULL))
);
CREATE UNIQUE INDEX one_active_consultant_per_branch ON users(branch_id) WHERE role = 'consultant' AND active;

CREATE TABLE universities (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  country text NOT NULL,
  created_by bigint REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX universities_name_uq ON universities (lower(name));

CREATE TABLE consultancies (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  created_by bigint REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX consultancies_name_uq ON consultancies (lower(name));

CREATE TABLE students (
  id bigserial PRIMARY KEY,
  source text NOT NULL CHECK (source IN ('internal','b2b')),
  branch_id smallint REFERENCES branches(id),
  consultancy_id bigint REFERENCES consultancies(id),
  owner_id bigint NOT NULL REFERENCES users(id),
  first_name text, family_name text, gender text, date_of_birth date,
  email citext NOT NULL, address text,
  ba_university text, ba_specialisation text, ba_duration text, ba_grade text,
  ma_university text, ma_specialisation text, ma_duration text, ma_grade text,
  xii_board text, xii_subjects text, xii_grade text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((source = 'internal') = (branch_id IS NOT NULL)),
  CHECK (source = 'b2b' OR consultancy_id IS NULL)
);
CREATE UNIQUE INDEX students_email_uq ON students (lower(btrim(email::text)));

CREATE TABLE applications (
  id bigserial PRIMARY KEY,
  student_id bigint NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  university_id bigint REFERENCES universities(id),
  custom_university_name text,
  custom_university_country text,
  programme_level text CHECK (programme_level IN ('masters','bachelors','top_up')),
  programme text,
  priority integer,
  intake text CHECK (intake IN ('autumn','spring')),
  year smallint,
  application_number text,
  submission_date date,
  status text CHECK (status IN ('applied','admitted','reserve','rejected','withdrawn','fees_paid','visa_granted','visa_rejected')),
  personal_code text,
  tuition_fee numeric(12,2),
  tuition_currency char(3),
  tuition_per text CHECK (tuition_per IN ('semester','year','programme')),
  scholarship text,
  created_by bigint NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((university_id IS NOT NULL) <> (custom_university_name IS NOT NULL))
);

CREATE TABLE activity_log (
  id bigserial PRIMARY KEY,
  student_id bigint REFERENCES students(id) ON DELETE CASCADE,
  actor_id bigint NOT NULL REFERENCES users(id),
  action text NOT NULL,
  detail jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- scs postgresstore schema
CREATE TABLE sessions (
  token text PRIMARY KEY,
  data bytea NOT NULL,
  expiry timestamptz NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- Indexes for admin filters and lookups
CREATE INDEX students_branch_idx       ON students (branch_id);
CREATE INDEX students_source_idx       ON students (source);
CREATE INDEX students_consultancy_idx  ON students (consultancy_id);
CREATE INDEX applications_student_idx  ON applications (student_id);
CREATE INDEX applications_uni_idx      ON applications (university_id);
CREATE INDEX applications_status_idx   ON applications (status);
CREATE INDEX applications_intake_idx   ON applications (intake, year);
CREATE INDEX applications_created_idx  ON applications (created_at);
CREATE INDEX activity_student_idx      ON activity_log (student_id, created_at);
CREATE INDEX students_search_trgm ON students USING gin (
  (lower(coalesce(first_name,'') || ' ' || coalesce(family_name,'') || ' ' || email::text)) gin_trgm_ops
);

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS activity_log;
DROP TABLE IF EXISTS applications;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS consultancies;
DROP TABLE IF EXISTS universities;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS branches;
-- extensions are left in place; other objects may depend on them
