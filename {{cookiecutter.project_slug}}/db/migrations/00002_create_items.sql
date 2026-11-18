-- +goose Up
CREATE TABLE items (
    id          UUID PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT items_name_uniq UNIQUE (name)
);

CREATE INDEX items_created_idx ON items (created_at DESC, id DESC);

-- +goose Down
DROP TABLE items;
