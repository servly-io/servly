// Package web embeds the built frontend (web/dist) into the server binary.
// `make build` runs the Vite build first; a plain `go build` embeds whatever
// web/dist holds, which may be only the placeholder.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend, rooted at dist/.
func Dist() fs.FS {
	f, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded; this cannot fail
	}
	return f
}
