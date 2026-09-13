// Package web embeds the built React dashboard so `bullpen server` ships as
// a single binary. The bundle is produced by `npm --prefix web run build`,
// which writes into internal/web/dist.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// all: is required so Vite's hashed asset files and the .gitkeep placeholder are
// both picked up. The directory always exists, so this compiles whether or not
// the dashboard has been built.
//
//go:embed all:dist
var bundle embed.FS

const placeholder = `bullpen dashboard is not built.

  make build-web      # or: npm --prefix web install && npm --prefix web run build

Then restart the server. The JSON API is unaffected and is already serving on
this port: /v1/summary, /v1/board, /v1/sessions, /v1/stream, /v1/context/retrieve.
`

// Built reports whether a real bundle was compiled in.
func Built() bool {
	_, err := fs.Stat(bundle, "dist/index.html")
	return err == nil
}

// Handler serves the dashboard, falling back to a single-page rewrite so client
// routes like /retrieval survive a reload.
func Handler() http.Handler {
	root, err := fs.Sub(bundle, "dist")
	if err != nil || !Built() {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(placeholder))
		})
	}

	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			files.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(root, path); err != nil {
			// unknown path with no file behind it: hand it to the SPA router
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
