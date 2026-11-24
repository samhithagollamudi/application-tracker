package server_test

// Changes requested in the 23/11 review document.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

func TestReviewWording(t *testing.T) {
	w := newWorld(t)
	login := w.app.Anon().Get("/login").Body
	if !strings.Contains(login, "Track every student and their application progress across all branches and the agent network") {
		t.Error("login tagline not updated")
	}
	if strings.Contains(login, "Forgot your password") {
		t.Error("forgot-password line should be gone")
	}

	page := w.chennai.Get(fmt.Sprintf("/students/%d", w.sChennai)).Body
	for _, gone := range []string{"<h2>Activity</h2>", "Owner:", "Chennai branch ·"} {
		if strings.Contains(page, gone) {
			t.Errorf("student page still shows %q", gone)
		}
	}
	if !strings.Contains(page, "Chennai · Consultant") {
		t.Error("role pill should read 'Chennai · Consultant'")
	}

	form := w.chennai.Get(fmt.Sprintf("/students/%d/applications/new", w.sChennai)).Body
	for _, want := range []string{"Tuition fee currency", `name="tuition_fee"`, `name="country"`,
		"For any non-partner university application, pick “Other” and type the university name."} {
		if !strings.Contains(form, want) {
			t.Errorf("application form missing %q", want)
		}
	}
	if strings.Contains(form, "Filled in from the university") {
		t.Error("country should be a dropdown, not an auto-filled box")
	}
}

func TestCountriesList(t *testing.T) {
	w := newWorld(t)
	// Seeded from the old fixed list; managed by admins only.
	if b := w.admin.Get("/admin/catalog?tab=countries").Body; !strings.Contains(b, ">Sweden<") || !strings.Contains(b, ">Estonia<") {
		t.Fatal("countries tab missing seeded countries")
	}
	if r := w.admin.Post("/admin/catalog/countries", url.Values{"name": {"Iceland"}}); r.Status != http.StatusSeeOther {
		t.Fatalf("add country: %d", r.Status)
	}
	if r := w.admin.Post("/admin/catalog/countries", url.Values{"name": {"iceland"}}); r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate country: %d", r.Status)
	}
	if r := w.chennai.Post("/admin/catalog/countries", url.Values{"name": {"Atlantis"}}); r.Status != http.StatusNotFound {
		t.Fatalf("consultant adding country: %d", r.Status)
	}
	// Universities must use a listed country.
	if r := w.admin.Post("/admin/catalog/universities", url.Values{"name": {"U of Nowhere"}, "country": {"Atlantis"}}); r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("university with unlisted country: %d", r.Status)
	}
	if r := w.admin.Post("/admin/catalog/universities", url.Values{"name": {"University of Iceland"}, "country": {"iceland"}}); r.Status != http.StatusSeeOther {
		t.Fatalf("university in new country: %d", r.Status)
	}
	if n := w.count(t, `SELECT count(*) FROM universities WHERE name = 'University of Iceland' AND country = 'Iceland'`); n != 1 {
		t.Fatal("university country not stored with the list spelling")
	}
	// In-use countries can't be removed; unused ones can.
	var sweden, malta int64
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM countries WHERE name = 'Sweden'`).Scan(&sweden)
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM countries WHERE name = 'Malta'`).Scan(&malta)
	w.admin.Post(fmt.Sprintf("/admin/catalog/countries/%d/delete", sweden), nil)
	w.admin.Post(fmt.Sprintf("/admin/catalog/countries/%d/delete", malta), nil)
	if n := w.count(t, `SELECT count(*) FROM countries WHERE name IN ('Sweden', 'Malta')`); n != 1 {
		t.Fatalf("expected only Sweden (in use) to remain, got %d", n)
	}
}

func TestApplicationCountry(t *testing.T) {
	w := newWorld(t)
	add := func(v url.Values) testapp.Response {
		return w.chennai.Post(fmt.Sprintf("/students/%d/applications/new", w.sChennai), v)
	}
	// One-off university: country must come from the list (any case).
	if r := add(url.Values{"uni_other": {"Tiny Poly"}, "country": {"Atlantis"}}); r.Status != http.StatusUnprocessableEntity ||
		!strings.Contains(r.Body, "Pick a country from the list") {
		t.Fatalf("unlisted country accepted: %d", r.Status)
	}
	if r := add(url.Values{"uni_other": {"Tiny Poly"}, "country": {"estonia"}}); r.Status != http.StatusSeeOther {
		t.Fatalf("one-off with listed country: %d", r.Status)
	}
	if n := w.count(t, `SELECT count(*) FROM applications WHERE custom_university_name = 'Tiny Poly' AND custom_university_country = 'Estonia'`); n != 1 {
		t.Fatal("one-off country not stored with the list spelling")
	}
	// A list university always keeps its own country, whatever the dropdown said.
	add(url.Values{"university_id": {fmt.Sprint(w.uniLund)}, "country": {"Finland"}})
	if n := w.count(t, `SELECT count(*) FROM applications a JOIN universities u ON u.id = a.university_id
		WHERE a.student_id = $1 AND u.country = 'Sweden'`, w.sChennai); n != 2 {
		t.Fatalf("list university applications = %d, want 2", n)
	}
	// The university search is limited to the chosen country.
	r := w.chennai.Get("/catalog/universities/search?country=Finland&uni_q=")
	if !strings.Contains(r.Body, "Aalto University") || strings.Contains(r.Body, "Lund University") {
		t.Error("search not filtered by country")
	}
	if r := w.chennai.Get("/catalog/universities/search?uni_q=lund"); !strings.Contains(r.Body, "Lund University") {
		t.Error("search without a country should cover all")
	}
}

func TestStudentApplicationFilters(t *testing.T) {
	w := newWorld(t)
	// Priya: Lund (no level/intake set except autumn 2027 from newWorld) + Aalto bachelors spring 2028.
	w.app.Exec(`UPDATE applications SET programme_level = 'masters', intake = 'autumn', year = 2027 WHERE id = $1`, w.aChennai)
	w.chennai.Post(fmt.Sprintf("/students/%d/applications/new", w.sChennai), url.Values{
		"uni_other": {"aalto university"}, "programme_level": {"bachelors"}, "intake": {"spring"}, "year": {"2028"},
	})
	has := func(q string) bool {
		return strings.Contains(w.chennai.Get("/students?"+q).Body, " · priya@example.com</div>")
	}
	cases := map[string]bool{
		"level=masters":                         true,
		"level=masters&intake=autumn&year=2027": true,
		"level=bachelors&year=2028":             true,
		"level=masters&intake=spring":           false, // no single application is a spring master's
		"level=masters&year=2028":               false,
		"intake=autumn&intake=spring&year=2026": false,
		"level=top_up":                          false,
	}
	for q, want := range cases {
		if got := has(q); got != want {
			t.Errorf("?%s: shows priya = %v, want %v", q, got, want)
		}
	}
	// The agent network gets the same filters.
	if b := w.agent.Get("/students").Body; !strings.Contains(b, `name="level"`) || !strings.Contains(b, `name="year"`) || !strings.Contains(b, `name="intake"`) {
		t.Error("agent network lacks the new filters")
	}
}
