// Package static embeds CSS, JS and images served under /static/.
package static

import (
	"embed"
	"net/http"
)

//go:embed *.css *.js *.png
var files embed.FS

// Handler serves the embedded files. Mount it with http.StripPrefix("/static/", ...).
func Handler() http.Handler {
	fs := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files are embedded in the binary, so they only change on deploy.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		fs.ServeHTTP(w, r)
	})
}
