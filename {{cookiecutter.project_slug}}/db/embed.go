// Package db embeds the goose SQL migrations.
package db

import (
	"embed"
	"io/fs"
)

// Schema is the Postgres schema of the service: created by 00001_init.sql (and
// by the migration bootstrap), it holds the tables and the goose version table
// and is the search_path of every connection.
const Schema = "{{ cookiecutter.db_schema }}"

//go:embed migrations/*.sql
var embedded embed.FS

// Migrations returns the migrations filesystem rooted at db/migrations.
func Migrations() fs.FS {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic(err) // unreachable: the directory is embedded at compile time
	}
	return sub
}
