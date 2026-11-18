-- name: CreateItem :one
INSERT INTO items (id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE id = $1;

-- name: GetItemForUpdate :one
SELECT * FROM items WHERE id = $1 FOR UPDATE;

-- name: ListItems :many
SELECT * FROM items
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateItem :one
UPDATE items
SET name        = coalesce(sqlc.narg(name), name),
    description = coalesce(sqlc.narg(description), description),
    updated_at  = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteItem :execrows
DELETE FROM items WHERE id = $1;
