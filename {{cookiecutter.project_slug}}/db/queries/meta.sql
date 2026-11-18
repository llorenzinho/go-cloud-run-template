-- Queries about the database itself. CurrentSchema backs the readiness check:
-- it fails when the search_path does not resolve to the service schema.

-- name: CurrentSchema :one
SELECT coalesce(current_schema(), '')::text;
