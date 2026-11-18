-- +goose Up
-- The schema of the service. The migration runner also creates it before goose
-- starts, because the goose version table lives in it.
CREATE SCHEMA IF NOT EXISTS {{ cookiecutter.db_schema }};

-- +goose Down
-- Fails while the schema still has objects: drop them in later migrations first.
DROP SCHEMA IF EXISTS {{ cookiecutter.db_schema }} RESTRICT;
