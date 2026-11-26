// Package db holds the SQL migrations (goose) and queries (sqlc input).
// The migrations are embedded in the server binary, so the server can
// apply them at start without extra files in the Docker image.
package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var files embed.FS

// Migrations returns the migration files, with db/migrations as the root.
func Migrations() fs.FS {
	sub, err := fs.Sub(files, "migrations")
	if err != nil {
		panic(err) // the path is a constant, so this cannot happen at run time
	}
	return sub
}
