// Package webui serves the embedded kapibara web dashboard (built React SPA).
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

var (
	assets fs.FS
	index  []byte
)

func init() {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	assets = sub
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}
	index = b
}

// Handler serves built static assets (JS/CSS under /assets) and falls back to
// index.html for all other paths so client-side routing works.
func Handler() http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve real static files (assets, favicon, etc.).
		if r.URL.Path != "/" && !strings.HasSuffix(r.URL.Path, "/") {
			if f, err := assets.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
