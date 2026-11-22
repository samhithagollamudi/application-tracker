package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

func TestHardening(t *testing.T) {
	app := testapp.New(t)
	c := app.Anon()
	r := c.Get("/login")
	for h, want := range map[string]string{
		"Content-Security-Policy": "script-src 'self'",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"X-Robots-Tag":            "noindex",
	} {
		if !strings.Contains(r.Header.Get(h), want) {
			t.Errorf("%s = %q, want it to contain %q", h, r.Header.Get(h), want)
		}
	}
	if r.Header.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
	if r := c.Get("/robots.txt"); !strings.Contains(r.Body, "Disallow: /") {
		t.Error("robots.txt should disallow everything")
	}
	if r := c.Get("/healthz"); r.Status != http.StatusOK || r.Body != "ok" {
		t.Errorf("healthz: %d %q", r.Status, r.Body)
	}
	// CSRF failures get the styled page, not a bare string.
	r = c.PostRaw("/login", url.Values{"email": {"a@b.c"}, "password": {"x"}})
	if r.Status != http.StatusForbidden || !strings.Contains(r.Body, "Please try again") {
		t.Errorf("CSRF failure page: %d", r.Status)
	}
	// Oversized bodies are rejected before they reach handlers.
	big := url.Values{"email": {strings.Repeat("a", 2<<20)}}
	if r := c.Post("/login", big); r.Status == http.StatusSeeOther || r.Status == http.StatusOK {
		t.Errorf("2 MB body accepted: %d", r.Status)
	}
}
