-- Admin-maintained list of countries for the application form's Country
-- dropdown and the university list (replaces the hard-coded list).

-- +goose Up
CREATE TABLE countries (
  id smallserial PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX countries_name_uq ON countries (lower(name));

INSERT INTO countries (name)
SELECT DISTINCT ON (lower(n)) n FROM (
  SELECT unnest(ARRAY['Austria','Belgium','Czechia','Denmark','Estonia','Finland','France','Germany','Hungary',
    'Ireland','Italy','Latvia','Lithuania','Luxembourg','Malta','Netherlands','Norway','Poland','Portugal',
    'Spain','Sweden','Switzerland','UK']) AS n
  UNION SELECT country FROM universities
  UNION SELECT custom_university_country FROM applications WHERE custom_university_country IS NOT NULL
) x
ORDER BY lower(n), n;

-- +goose Down
DROP TABLE countries;
