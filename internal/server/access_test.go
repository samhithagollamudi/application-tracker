package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

type world struct {
	app                        *testapp.App
	admin, chennai, hyd, agent *testapp.Client
	sChennai, sHyd, sB2B       int64 // student ids
	aChennai, aHyd, aB2B       int64 // one application id each
	uniLund                    int64
}

func studentID(t *testing.T, r testapp.Response) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(r.Location, "/students/%d", &id); err != nil {
		t.Fatalf("expected redirect to a student, got %d %q\n%s", r.Status, r.Location, r.Body)
	}
	return id
}

func (w *world) firstAppID(t *testing.T, sid int64) int64 {
	t.Helper()
	var id int64
	if err := w.app.Pool.QueryRow(context.Background(),
		`SELECT id FROM applications WHERE student_id = $1 ORDER BY id LIMIT 1`, sid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newWorld(t *testing.T) *world {
	t.Helper()
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	app.CreateUser("Chennai consultant", "chennai@sny.test", "consultant", "Chennai")
	app.CreateUser("Hyderabad consultant", "hyd@sny.test", "consultant", "Hyderabad")
	app.CreateUser("Agent network", "agent@sny.test", "agent_network", "")
	app.Exec(`INSERT INTO universities (name, country) VALUES ('Lund University', 'Sweden'), ('Aalto University', 'Finland')`)
	app.Exec(`INSERT INTO consultancies (name) VALUES ('Alpha Overseas')`)

	w := &world{app: app,
		admin: app.Login("admin@sny.test"), chennai: app.Login("chennai@sny.test"),
		hyd: app.Login("hyd@sny.test"), agent: app.Login("agent@sny.test")}
	_ = app.Pool.QueryRow(context.Background(), `SELECT id FROM universities WHERE name = 'Lund University'`).Scan(&w.uniLund)
	lund := fmt.Sprint(w.uniLund)

	w.sChennai = studentID(t, w.chennai.Post("/students/new", url.Values{
		"first_name": {"Priya"}, "family_name": {"Rao"}, "email": {"priya@example.com"},
		"university_id": {lund}, "status": {"applied"}, "intake": {"autumn"}, "year": {"2027"},
	}))
	w.sHyd = studentID(t, w.hyd.Post("/students/new", url.Values{
		"first_name": {"Rahul"}, "email": {"rahul@example.com"}, "university_id": {lund},
	}))
	var alpha int64
	_ = app.Pool.QueryRow(context.Background(), `SELECT id FROM consultancies WHERE name = 'Alpha Overseas'`).Scan(&alpha)
	w.sB2B = studentID(t, w.agent.Post("/students/new", url.Values{
		"first_name": {"Kavya"}, "email": {"kavya@example.com"}, "consultancy_id": {fmt.Sprint(alpha)},
		"university_id": {lund},
	}))
	w.aChennai, w.aHyd, w.aB2B = w.firstAppID(t, w.sChennai), w.firstAppID(t, w.sHyd), w.firstAppID(t, w.sB2B)
	return w
}

func TestAccessMatrix(t *testing.T) {
	w := newWorld(t)

	type expect struct {
		name    string
		c       *testapp.Client
		visible map[int64]bool // student id -> visible
	}
	all := map[int64]bool{w.sChennai: true, w.sHyd: true, w.sB2B: true}
	cases := []expect{
		{"admin", w.admin, all},
		{"chennai", w.chennai, map[int64]bool{w.sChennai: true}},
		{"hyderabad", w.hyd, map[int64]bool{w.sHyd: true}},
		{"agent network", w.agent, map[int64]bool{w.sB2B: true}},
	}
	emails := map[int64]string{w.sChennai: "priya@example.com", w.sHyd: "rahul@example.com", w.sB2B: "kavya@example.com"}
	appOf := map[int64]int64{w.sChennai: w.aChennai, w.sHyd: w.aHyd, w.sB2B: w.aB2B}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := tc.c.Get("/students")
			if list.Status != http.StatusOK {
				t.Fatalf("list: %d", list.Status)
			}
			// Search by email must not leak other students either.
			for sid, email := range emails {
				search := tc.c.Get("/students?q=" + url.QueryEscape(email))
				for _, body := range []string{list.Body, search.Body} {
					if got := strings.Contains(body, " · "+email+"</div>"); got != tc.visible[sid] { // the row line, not the echoed search box
						t.Errorf("list shows %s = %v, want %v", email, got, tc.visible[sid])
					}
				}
			}
			for sid := range all {
				want := http.StatusNotFound
				if tc.visible[sid] {
					want = http.StatusOK
				}
				aid := appOf[sid]
				check := func(what string, got int, wantOK int) {
					t.Helper()
					w := want
					if want == http.StatusOK {
						w = wantOK
					}
					if got != w {
						t.Errorf("%s on student %d: status %d, want %d", what, sid, got, w)
					}
				}
				check("detail", tc.c.Get(fmt.Sprintf("/students/%d", sid)).Status, http.StatusOK)
				check("edit form", tc.c.Get(fmt.Sprintf("/students/%d/edit", sid)).Status, http.StatusOK)
				check("add-application form", tc.c.Get(fmt.Sprintf("/students/%d/applications/new", sid)).Status, http.StatusOK)
				check("application edit form", tc.c.Get(fmt.Sprintf("/applications/%d/edit", aid)).Status, http.StatusOK)
				if !tc.visible[sid] {
					// Writes by guessing IDs must be refused and change nothing.
					check("edit POST", tc.c.Post(fmt.Sprintf("/students/%d/edit", sid), url.Values{"email": {"hijack@example.com"}, "first_name": {"Hijacked"}}).Status, 0)
					check("add application POST", tc.c.Post(fmt.Sprintf("/students/%d/applications/new", sid), url.Values{"university_id": {fmt.Sprint(w.uniLund)}}).Status, 0)
					check("status POST", tc.c.Post(fmt.Sprintf("/applications/%d/status", aid), url.Values{"status": {"rejected"}}).Status, 0)
					check("application edit POST", tc.c.Post(fmt.Sprintf("/applications/%d/edit", aid), url.Values{"uni_other": {"Hijack U"}}).Status, 0)
					check("delete POST", tc.c.Post(fmt.Sprintf("/applications/%d/delete", aid), nil).Status, 0)
				}
			}
		})
	}

	// Nothing above may have changed out-of-scope data.
	var n int
	_ = w.app.Pool.QueryRow(context.Background(), `SELECT count(*) FROM students WHERE email = 'hijack@example.com' OR first_name = 'Hijacked'`).Scan(&n)
	if n != 0 {
		t.Fatal("an out-of-scope student was modified")
	}
	_ = w.app.Pool.QueryRow(context.Background(), `SELECT count(*) FROM applications`).Scan(&n)
	if n != 3 {
		t.Fatalf("applications count = %d, want 3 (none added or deleted)", n)
	}
	_ = w.app.Pool.QueryRow(context.Background(), `SELECT count(*) FROM applications WHERE status = 'rejected' OR custom_university_name IS NOT NULL`).Scan(&n)
	if n != 0 {
		t.Fatal("an out-of-scope application was modified")
	}
}

func TestRoleOnlyRoutes(t *testing.T) {
	w := newWorld(t)
	// Admin doesn't create students.
	if r := w.admin.Get("/students/new"); r.Status != http.StatusNotFound {
		t.Errorf("admin /students/new: %d", r.Status)
	}
	// Only admin moves students / sees all applications.
	for _, c := range []*testapp.Client{w.chennai, w.hyd, w.agent} {
		if r := c.Post(fmt.Sprintf("/students/%d/move", w.sChennai), url.Values{"branch_id": {"2"}}); r.Status != http.StatusNotFound {
			t.Errorf("non-admin move: %d", r.Status)
		}
		if r := c.Get("/applications"); r.Status != http.StatusNotFound {
			t.Errorf("non-admin /applications: %d", r.Status)
		}
	}
}

func TestDuplicateEmailMessages(t *testing.T) {
	w := newWorld(t)
	// Own student, different case and spaces: "already exists" + link.
	r := w.chennai.Post("/students/new", url.Values{"email": {"  PRIYA@example.com "}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already exists") ||
		!strings.Contains(r.Body, fmt.Sprintf(`href="/students/%d"`, w.sChennai)) {
		t.Fatalf("visible duplicate: %d\n%s", r.Status, r.Body)
	}
	// Another branch's student: generic message, no link, no details.
	r = w.chennai.Post("/students/new", url.Values{"email": {"rahul@example.com"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already registered. Contact the admin") {
		t.Fatalf("hidden duplicate: %d", r.Status)
	}
	if strings.Contains(r.Body, fmt.Sprintf("/students/%d", w.sHyd)) || strings.Contains(r.Body, "Rahul") {
		t.Fatal("hidden duplicate leaks the other student")
	}
	// Editing to a taken email is refused too.
	r = w.agent.Post(fmt.Sprintf("/students/%d/edit", w.sB2B), url.Values{"email": {"priya@example.com"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "already registered") {
		t.Fatalf("edit to duplicate: %d", r.Status)
	}
	// Email is the only required field and must be valid.
	r = w.chennai.Post("/students/new", url.Values{"first_name": {"No email"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "email is required") {
		t.Fatalf("missing email: %d", r.Status)
	}
	r = w.chennai.Post("/students/new", url.Values{"email": {"not-an-email"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "valid email") {
		t.Fatalf("bad email: %d", r.Status)
	}
}
