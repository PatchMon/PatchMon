package server

import (
	"io/fs"
	"net/http"
)

// SPAHandler serves static files from the given FS, falling back to index.html for SPA routing.
// index.html is served with Cache-Control: no-cache so browsers always fetch the latest version.
// Hashed asset files (JS/CSS) rely on Vite content-hash filenames and can be cached long-term.
func SPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		if path == "" {
			path = "index.html"
		}

		f, err := fsys.Open(path)
		if err == nil {
			_ = f.Close()
			if path == "index.html" || path == "" {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// fall back to index.html for client-side routing
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
