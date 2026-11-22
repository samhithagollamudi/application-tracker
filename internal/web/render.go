// Package web holds rendering helpers shared by the feature handlers.
package web

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/ChandraShekar2001/application-tracker/internal/httpx"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

// Render buffers the component so a template error becomes a clean 500
// instead of a half-written page.
func Render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		slog.Error("render", "id", httpx.RequestID(r.Context()), "path", r.URL.Path, "err", err)
		Error(w, r, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func OK(w http.ResponseWriter, r *http.Request, c templ.Component) { Render(w, r, http.StatusOK, c) }

// Error renders the styled error page.
func Error(w http.ResponseWriter, r *http.Request, status int) {
	title, msg := "Something went wrong", "Please try again. If it keeps happening, tell the admin."
	switch status {
	case http.StatusNotFound:
		title, msg = "Not found", "It doesn’t exist, or you don’t have access to it."
	case http.StatusBadRequest:
		title, msg = "That didn’t work", "The request was not valid. Go back and check the form."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = pages.Error(status, title, msg).Render(r.Context(), w)
}

// ServerError logs err and renders a 500.
func ServerError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("handler", "id", httpx.RequestID(r.Context()), "method", r.Method, "path", r.URL.Path, "err", err)
	Error(w, r, http.StatusInternalServerError)
}

func NotFound(w http.ResponseWriter, r *http.Request) { Error(w, r, http.StatusNotFound) }
