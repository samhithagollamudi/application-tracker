// Package static embeds CSS, JS and images served under /static/.
package static

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
)

//go:embed *.css *.js *.png *.svg *.txt
var files embed.FS

// Version is a hash of all embedded files. Pages reference /static/x?v=Version,
// so a deploy with changed assets is never served from a stale browser cache.
var Version = func() string {
	h := sha256.New()
	_ = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(path)
		h.Write([]byte(path))
		h.Write(b)
		return err
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
}()

// Handler serves the embedded files. Mount it with http.StripPrefix("/static/", ...).
func Handler() http.Handler {
	fs := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// URLs carry ?v=Version, so versioned requests can be cached for a long time.
		if r.URL.Query().Get("v") == Version {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}
