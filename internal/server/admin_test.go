package server_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

func TestAdminApplicationsFilters(t *testing.T) {
	w := newWorld(t)
	// Make the data more varied.
	w.chennai.Post(fmt.Sprintf("/students/%d/applications/new", w.sChennai), url.Values{
		"uni_other": {"Tiny Polytechnic"}, "uni_other_country": {"Estonia"}, "status": {"admitted"}, "intake": {"spring"}, "year": {"2028"},
	})

	r := w.admin.Get("/applications")
	if r.Status != http.StatusOK {
		t.Fatalf("applications: %d", r.Status)
	}
	if !strings.Contains(r.Body, "Export CSV (4)") {
		t.Error("expected 4 applications in total")
	}
	cases := map[string][]string{ // query -> student emails expected (others must be absent)
		"source=b2b":         {"kavya@example.com"},
		"country=Estonia":    {"priya@example.com"},
		"status=admitted":    {"priya@example.com"},
		"intake=spring-2028": {"priya@example.com"},
		"q=tiny":             {"priya@example.com"},
		"q=" + url.QueryEscape("APP-"+fmt.Sprint(w.aHyd)): {"rahul@example.com"},
		"consultancy=none&q=rahul":                        {"rahul@example.com"},
		"from=2000-01-01&to=2000-01-02":                   {},
	}
	all := []string{"priya@example.com", "rahul@example.com", "kavya@example.com"}
	for q, want := range cases {
		body := w.admin.Get("/applications?" + q).Body
		for _, e := range all {
			wantIt := false
			for _, x := range want {
				wantIt = wantIt || x == e
			}
			if got := strings.Contains(body, `<div class="small muted">`+e+`</div>`); got != wantIt {
				t.Errorf("?%s: shows %s = %v, want %v", q, e, got, wantIt)
			}
		}
	}
	// The export link carries the same filters.
	if b := w.admin.Get("/applications?status=admitted").Body; !strings.Contains(b, `name="query" value="status=admitted"`) {
		t.Error("export link doesn't carry the filters")
	}
}

func TestCatalogAdmin(t *testing.T) {
	w := newWorld(t)
	r := w.admin.Post("/admin/catalog/universities", url.Values{"name": {"  Uppsala   University "}, "country": {"Sweden"}})
	if r.Status != http.StatusSeeOther {
		t.Fatalf("add university: %d", r.Status)
	}
	if n := w.count(t, `SELECT count(*) FROM universities WHERE name = 'Uppsala University' AND country = 'Sweden'`); n != 1 {
		t.Fatal("university not added")
	}
	r = w.admin.Post("/admin/catalog/universities", url.Values{"name": {"uppsala university"}, "country": {"Sweden"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already in the list") {
		t.Fatalf("duplicate university: %d", r.Status)
	}
	r = w.admin.Post("/admin/catalog/consultancies", url.Values{"name": {"Delta Global"}})
	if r.Status != http.StatusSeeOther {
		t.Fatalf("add consultancy: %d", r.Status)
	}
	// In-use entries can't be removed; unused ones can.
	w.admin.Post(fmt.Sprintf("/admin/catalog/universities/%d/delete", w.uniLund), nil)
	if n := w.count(t, `SELECT count(*) FROM universities WHERE id = $1`, w.uniLund); n != 1 {
		t.Fatal("in-use university was removed")
	}
	var up int64
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM universities WHERE name = 'Uppsala University'`).Scan(&up)
	w.admin.Post(fmt.Sprintf("/admin/catalog/universities/%d/delete", up), nil)
	if n := w.count(t, `SELECT count(*) FROM universities WHERE id = $1`, up); n != 0 {
		t.Fatal("unused university not removed")
	}
	// The agent's "Other" consultancy is marked as such.
	w.agent.Post("/students/new", url.Values{"email": {"x@example.com"}, "consultancy_id": {"__other"}, "consultancy_other": {"Zeta Abroad"}})
	if b := w.admin.Get("/admin/catalog?tab=consultancies").Body; !strings.Contains(b, "Other · Agent network") {
		t.Error("agent-added consultancy not marked")
	}
	// Non-admins can't use any of it.
	if r := w.chennai.Post("/admin/catalog/universities", url.Values{"name": {"Sneaky U"}, "country": {"Sweden"}}); r.Status != http.StatusNotFound {
		t.Fatalf("consultant adding university: %d", r.Status)
	}
}

var pwRe = regexp.MustCompile(`<code class="pw" id="newPw">([^<]+)</code>`)

func TestUserManagement(t *testing.T) {
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	admin := app.Login("admin@sny.test")
	var puneID string
	_ = app.Pool.QueryRow(t.Context(), `SELECT id::text FROM branches WHERE name = 'Pune'`).Scan(&puneID)

	// Create a consultant: password shown once, and it works.
	r := admin.Post("/admin/users", url.Values{"name": {"Pune consultant"}, "email": {"pune@sny.test"}, "role": {"consultant"}, "branch_id": {puneID}})
	m := pwRe.FindStringSubmatch(r.Body)
	if r.Status != http.StatusOK || m == nil {
		t.Fatalf("create user: %d", r.Status)
	}
	if r.Header.Get("Cache-Control") != "no-store" {
		t.Error("password page must not be cached")
	}
	pw := m[1]
	if strings.Contains(admin.Get("/admin/users").Body, pw) {
		t.Fatal("password shown a second time")
	}
	c := app.Anon()
	if r := c.Post("/login", url.Values{"email": {"pune@sny.test"}, "password": {pw}}); r.Status != http.StatusSeeOther {
		t.Fatalf("new user can't sign in: %d", r.Status)
	}
	if r := c.Get("/students"); !strings.Contains(r.Body, "Consultant · Pune") {
		t.Fatal("new consultant not in Pune")
	}

	// One active consultant per branch.
	r = admin.Post("/admin/users", url.Values{"name": {"Second"}, "email": {"pune2@sny.test"}, "role": {"consultant"}, "branch_id": {puneID}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already has an active consultant") {
		t.Fatalf("second consultant for Pune: %d", r.Status)
	}
	// Duplicate user ID.
	r = admin.Post("/admin/users", url.Values{"name": {"Dup"}, "email": {"PUNE@sny.test"}, "role": {"agent_network"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already taken") {
		t.Fatalf("duplicate user id: %d", r.Status)
	}

	var uid int64
	_ = app.Pool.QueryRow(t.Context(), `SELECT id FROM users WHERE email = 'pune@sny.test'`).Scan(&uid)

	// Reset: new password works, old one doesn't, existing session ends.
	r = admin.Post(fmt.Sprintf("/admin/users/%d/reset", uid), nil)
	m2 := pwRe.FindStringSubmatch(r.Body)
	if m2 == nil || m2[1] == pw {
		t.Fatal("reset didn't show a new password")
	}
	if r := c.Get("/students"); r.Location != "/login" {
		t.Fatal("session survived reset")
	}
	if r := app.Anon().Post("/login", url.Values{"email": {"pune@sny.test"}, "password": {pw}}); r.Status != http.StatusUnauthorized {
		t.Fatal("old password still works")
	}
	if r := app.Anon().Post("/login", url.Values{"email": {"pune@sny.test"}, "password": {m2[1]}}); r.Status != http.StatusSeeOther {
		t.Fatal("new password doesn't work")
	}

	// Deactivate frees the branch; reactivation is blocked while another consultant holds it.
	admin.Post(fmt.Sprintf("/admin/users/%d/toggle", uid), nil)
	r = admin.Post("/admin/users", url.Values{"name": {"Replacement"}, "email": {"pune3@sny.test"}, "role": {"consultant"}, "branch_id": {puneID}})
	if r.Status != http.StatusOK {
		t.Fatalf("replacement consultant: %d", r.Status)
	}
	admin.Post(fmt.Sprintf("/admin/users/%d/toggle", uid), nil)
	if n := 0; app.Pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE id = $1 AND active`, uid).Scan(&n) == nil && n != 0 {
		t.Fatal("reactivated a second consultant for the branch")
	}

	// Admin can't deactivate themselves.
	var me int64
	_ = app.Pool.QueryRow(t.Context(), `SELECT id FROM users WHERE email = 'admin@sny.test'`).Scan(&me)
	admin.Post(fmt.Sprintf("/admin/users/%d/toggle", me), nil)
	if r := admin.Get("/admin/users"); r.Status != http.StatusOK {
		t.Fatal("admin locked themselves out")
	}
}
