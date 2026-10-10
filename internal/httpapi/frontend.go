package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// frontend serves the single-page application.
func frontend(dist fs.FS) http.Handler {
	if dist == nil {
		return http.HandlerFunc(notBuilt)
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return http.HandlerFunc(notBuilt)
	}
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			serveIndex(w, r, dist)
			return
		}
		if st, err := fs.Stat(dist, name); err != nil || st.IsDir() {
			if errors.Is(err, fs.ErrNotExist) && path.Ext(name) != "" {
				// A missing asset is a 404, not the app shell.
				http.NotFound(w, r)
				return
			}
			serveIndex(w, r, dist)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, dist, "index.html")
}

func notBuilt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("servly: the web frontend is not built into this binary. Run `make build`.\n"))
}
