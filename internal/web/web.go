// Package web serves the built Vite frontend. In dev the UI runs on Vite's
// :5173 and proxies /api to this server; in production both ship from one process.
//
// The embed directive lives in the root main package (which can reach web/dist)
// and the resulting fs.FS is handed to Handler.
package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// Handler serves the SPA rooted at the given embedded dist filesystem. Any
// non-/api path that maps to a real file returns it; everything else falls back
// to index.html so client-side routing works on deep links.
func Handler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// If the path doesn't refer to a real embedded asset, serve index.html so
		// deep links like /asn/13335 resolve via client-side routing.
		if _, err := fs.Stat(distFS, path); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
