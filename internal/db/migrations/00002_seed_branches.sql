-- +goose Up
INSERT INTO branches (name) VALUES
  ('Chennai'), ('Hyderabad'), ('Vijayawada'), ('Bengaluru'), ('Pune');

-- +goose Down
DELETE FROM branches WHERE name IN ('Chennai', 'Hyderabad', 'Vijayawada', 'Bengaluru', 'Pune');
