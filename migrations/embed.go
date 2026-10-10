// Package migrations holds the versioned SQL migrations.
// docs/docs/domain-model.md.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed sqlite/*.sql postgres/*.sql
var files embed.FS

// SQLite returns the SQLite migration set.
func SQLite() fs.FS { return sub("sqlite") }

// Postgres returns the PostgreSQL migration set.
func Postgres() fs.FS { return sub("postgres") }

func sub(dir string) fs.FS {
	f, err := fs.Sub(files, dir)
	if err != nil {
		panic(err) // the directory is embedded; this cannot fail
	}
	return f
}
