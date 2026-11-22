package auth_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := auth.HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format %q", h)
	}
	if ok, err := auth.CheckPassword(h, "s3cret"); err != nil || !ok {
		t.Fatalf("correct password rejected: %v", err)
	}
	if ok, _ := auth.CheckPassword(h, "S3cret"); ok {
		t.Fatal("wrong password accepted")
	}
}

func TestGeneratePassword(t *testing.T) {
	a, b := auth.GeneratePassword(), auth.GeneratePassword()
	if a == b || len(a) != 23 || strings.Count(a, "-") != 3 {
		t.Fatalf("bad generated passwords %q %q", a, b)
	}
}

func TestLoginFlow(t *testing.T) {
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	app.CreateUser("Chennai", "chennai@sny.test", "consultant", "Chennai")

	anon := app.Anon()
	if r := anon.Get("/students"); r.Status != http.StatusSeeOther || r.Location != "/login" {
		t.Fatalf("anonymous /students: %d %q, want redirect to /login", r.Status, r.Location)
	}

	// Wrong password
	r := anon.Post("/login", url.Values{"email": {"admin@sny.test"}, "password": {"nope"}})
	if r.Status != http.StatusUnauthorized || !strings.Contains(r.Body, "don’t match") {
		t.Fatalf("bad password: %d", r.Status)
	}
	// Unknown user gives the same message
	r = anon.Post("/login", url.Values{"email": {"ghost@sny.test"}, "password": {"nope"}})
	if r.Status != http.StatusUnauthorized || !strings.Contains(r.Body, "don’t match") {
		t.Fatalf("unknown user: %d", r.Status)
	}

	// Email is case-insensitive; admin lands on /applications, consultant on /students
	admin := app.Login("ADMIN@sny.test")
	if r := admin.Get("/"); r.Location != "/applications" {
		t.Fatalf("admin home redirect = %q", r.Location)
	}
	cons := app.Login("chennai@sny.test")
	if r := cons.Get("/"); r.Location != "/students" {
		t.Fatalf("consultant home redirect = %q", r.Location)
	}
	if r := cons.Get("/students"); !strings.Contains(r.Body, "Consultant · Chennai") {
		t.Fatal("role pill missing from shell")
	}

	// Logout ends the session
	if r := cons.Post("/logout", nil); r.Status != http.StatusSeeOther {
		t.Fatalf("logout: %d", r.Status)
	}
	if r := cons.Get("/students"); r.Location != "/login" {
		t.Fatal("still signed in after logout")
	}
}

func TestCSRFRequired(t *testing.T) {
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	c := app.Login("admin@sny.test")
	if r := c.PostRaw("/logout", url.Values{}); r.Status != http.StatusForbidden {
		t.Fatalf("POST without CSRF token: %d, want 403", r.Status)
	}
	if r := c.PostRaw("/logout", url.Values{"csrf_token": {"forged"}}); r.Status != http.StatusForbidden {
		t.Fatalf("POST with wrong CSRF token: %d, want 403", r.Status)
	}
}

func TestDeactivationEndsSessions(t *testing.T) {
	app := testapp.New(t)
	id := app.CreateUser("Pune", "pune@sny.test", "consultant", "Pune")
	c := app.Login("pune@sny.test")
	app.Exec(`UPDATE users SET active = false, auth_version = auth_version + 1 WHERE id = $1`, id)
	if r := c.Get("/students"); r.Location != "/login" {
		t.Fatalf("deactivated user still signed in: %d", r.Status)
	}
	r := app.Anon().Post("/login", url.Values{"email": {"pune@sny.test"}, "password": {testapp.Password}})
	if !strings.Contains(r.Body, "deactivated") {
		t.Fatal("deactivated user could sign in")
	}
}

func TestPasswordResetEndsSessions(t *testing.T) {
	app := testapp.New(t)
	id := app.CreateUser("Pune", "pune@sny.test", "consultant", "Pune")
	c := app.Login("pune@sny.test")
	app.Exec(`UPDATE users SET auth_version = auth_version + 1 WHERE id = $1`, id)
	if r := c.Get("/students"); r.Location != "/login" {
		t.Fatal("session survived password reset")
	}
}

func TestLoginRateLimit(t *testing.T) {
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	c := app.Anon()
	for i := 0; i < 5; i++ {
		c.Post("/login", url.Values{"email": {"admin@sny.test"}, "password": {"wrong"}})
	}
	// Even the right password is refused while locked out.
	r := c.Post("/login", url.Values{"email": {"admin@sny.test"}, "password": {testapp.Password}})
	if r.Status != http.StatusUnauthorized || !strings.Contains(r.Body, "Too many failed attempts") {
		t.Fatalf("expected lockout, got %d", r.Status)
	}
}
