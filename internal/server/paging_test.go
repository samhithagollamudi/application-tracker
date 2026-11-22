package server_test

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ChandraShekar2001/application-tracker/internal/testapp"
)

// bulk inserts n internal Chennai students with one application each, with
// distinct updated_at so the order is deterministic (student i newest = n).
func bulk(t *testing.T, app *testapp.App, n int, owner string) {
	t.Helper()
	app.Exec(`INSERT INTO universities (name, country) VALUES ('Bulk University', 'Sweden') ON CONFLICT DO NOTHING`)
	app.Exec(`
		WITH ins AS (
			INSERT INTO students (source, branch_id, owner_id, first_name, family_name, email, updated_at)
			SELECT 'internal', (SELECT id FROM branches WHERE name = 'Chennai'), (SELECT id FROM users WHERE email = $2),
			       'Bulk', lpad(g::text, 5, '0'), 'bulk' || g || '@example.com', now() - (($1 - g) || ' seconds')::interval
			FROM generate_series(1, $1) g
			RETURNING id, updated_at
		)
		INSERT INTO applications (student_id, university_id, status, created_by, updated_at)
		SELECT id, (SELECT id FROM universities WHERE name = 'Bulk University'), 'applied',
		       (SELECT id FROM users WHERE email = $2), updated_at FROM ins`, n, owner)
}

var rowEmail = regexp.MustCompile(`bulk(\d+)@example\.com`)

func emails(body string) []string {
	var out []string
	for _, m := range rowEmail.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestPagination(t *testing.T) {
	app := testapp.New(t)
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	app.CreateUser("Chennai", "chennai@sny.test", "consultant", "Chennai")
	bulk(t, app, 120, "chennai@sny.test")
	admin, cons := app.Login("admin@sny.test"), app.Login("chennai@sny.test")

	// Students: 50 per page, newest first, 3 pages.
	p1 := cons.Get("/students")
	got := emails(p1.Body)
	if len(got) != 50 || got[0] != "120" || got[49] != "71" {
		t.Fatalf("students page 1: %d rows, first %v", len(got), got[:min(3, len(got))])
	}
	if !strings.Contains(p1.Body, "1–50 of 120 students") || !strings.Contains(p1.Body, `href="/students?page=3"`) {
		t.Error("pager missing range or last-page link")
	}
	p2 := emails(cons.Get("/students?page=2").Body)
	if len(p2) != 50 || p2[0] != "70" || p2[49] != "21" {
		t.Fatalf("students page 2 wrong: first %s last %s", p2[0], p2[len(p2)-1])
	}
	p3 := cons.Get("/students?page=3")
	if g := emails(p3.Body); len(g) != 20 || g[19] != "1" || !strings.Contains(p3.Body, "101–120 of 120") {
		t.Fatalf("students page 3: %d rows", len(g))
	}
	// Past the end clamps to the last page; junk means page 1.
	if g := emails(cons.Get("/students?page=99").Body); len(g) != 20 {
		t.Errorf("page 99 should show the last page, got %d rows", len(g))
	}
	if g := emails(cons.Get("/students?page=abc").Body); len(g) != 50 || g[0] != "120" {
		t.Error("invalid page should show page 1")
	}
	// A search that fits on one page shows the count and no page links.
	if f := cons.Get("/students?q=bulk1"); !strings.Contains(f.Body, ">32 students<") || strings.Contains(f.Body, `class="pages"`) {
		t.Error("one-page search wrong (bulk1, bulk10-19, bulk100-120 = 32)")
	}
	// Filters survive in page links.
	if f := cons.Get("/students?q=bulk&page=2"); !strings.Contains(f.Body, "51–100 of 120") ||
		!strings.Contains(f.Body, `href="/students?q=bulk&amp;page=3"`) || !strings.Contains(f.Body, `href="/students?q=bulk"`) {
		t.Error("page links lost the search filter")
	}

	// Applications (admin): same paging, stats still over everything.
	a1 := admin.Get("/applications")
	if !strings.Contains(a1.Body, "1–50 of 120 applications") || !strings.Contains(a1.Body, ">120</strong>") {
		t.Error("applications pager/stats wrong")
	}
	if g := emails(admin.Get("/applications?page=3").Body); len(g) != 20 {
		t.Errorf("applications page 3: %d rows", len(g))
	}
}

func TestBackgroundExport(t *testing.T) {
	app := testapp.New(t, testapp.WithExportDirectLimit(10))
	app.CreateUser("Admin", "admin@sny.test", "admin", "")
	app.CreateUser("Admin two", "admin2@sny.test", "admin", "")
	app.CreateUser("Chennai", "chennai@sny.test", "consultant", "Chennai")
	bulk(t, app, 25, "chennai@sny.test")
	admin := app.Login("admin@sny.test")

	// Nothing pending: the notice poll tells htmx to stop polling.
	if r := admin.Get("/exports/notify"); r.Status != 286 {
		t.Fatalf("idle notify: %d, want 286", r.Status)
	}

	// Small result sets (<= limit) still download immediately.
	r := admin.Post("/applications/export", url.Values{"query": {"q=bulk2"}}) // bulk2, bulk20-25 = 7 rows
	if r.Status != http.StatusOK || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("small export: %d", r.Status)
	}

	// Large ones are queued: redirect back to the same filters with a notice.
	r = admin.Post("/applications/export", url.Values{"query": {"status=applied&page=2"}})
	if r.Status != http.StatusSeeOther || r.Location != "/applications?status=applied" {
		t.Fatalf("large export: %d %q", r.Status, r.Location)
	}
	if page := admin.Get(r.Location); !strings.Contains(page.Body, "Preparing your export of 25 applications") {
		t.Error("missing 'preparing' notice")
	}
	// Wait for the background job.
	var id int64
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status string
		_ = app.Pool.QueryRow(context.Background(), `SELECT id, status FROM exports`).Scan(&id, &status)
		if status == "done" {
			break
		}
		if status == "failed" || time.Now().After(deadline) {
			t.Fatalf("export ended as %q", status)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The next poll shows the notice once, with a download link.
	n := admin.Get("/exports/notify")
	if n.Status != http.StatusOK || !strings.Contains(n.Body, "export of 25 applications is ready") ||
		!strings.Contains(n.Body, fmt.Sprintf(`href="/exports/%d/download"`, id)) {
		t.Fatalf("ready notice: %d %s", n.Status, n.Body)
	}
	if n := admin.Get("/exports/notify"); n.Status != 286 {
		t.Errorf("notice repeated / polling not stopped: %d", n.Status)
	}
	if l := admin.Get("/exports"); !strings.Contains(l.Body, "Ready") || !strings.Contains(l.Body, "Download") {
		t.Error("exports page doesn't list the finished export")
	}

	// Download has every row, decompressed, with the BOM + header.
	d := admin.Get(fmt.Sprintf("/exports/%d/download", id))
	if d.Status != http.StatusOK || !strings.Contains(d.Header.Get("Content-Disposition"), "applications-") {
		t.Fatalf("download: %d", d.Status)
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(d.Body, "\uFEFF"))).ReadAll()
	if err != nil || len(recs) != 26 {
		t.Fatalf("downloaded %d rows (%v), want header + 25", len(recs), err)
	}

	// Stored compressed in the database.
	var file []byte
	_ = app.Pool.QueryRow(context.Background(), `SELECT file FROM exports WHERE id = $1`, id).Scan(&file)
	zr, err := gzip.NewReader(strings.NewReader(string(file)))
	if err != nil {
		t.Fatalf("stored file is not gzip: %v", err)
	}
	if b, _ := io.ReadAll(zr); !strings.Contains(string(b), "Application ID") {
		t.Error("stored file content wrong")
	}

	// Only the admin who made it can download it; non-admins can't reach exports at all.
	if r := app.Login("admin2@sny.test").Get(fmt.Sprintf("/exports/%d/download", id)); r.Status != http.StatusNotFound {
		t.Errorf("other admin download: %d", r.Status)
	}
	cons := app.Login("chennai@sny.test")
	for _, p := range []string{"/exports", "/exports/notify", fmt.Sprintf("/exports/%d/download", id)} {
		if r := cons.Get(p); r.Status != http.StatusNotFound {
			t.Errorf("consultant %s: %d", p, r.Status)
		}
	}

	// Asking for the same export while one is still running reuses that job.
	app.Exec(`INSERT INTO exports (user_id, query, status) SELECT id, 'q=bulk', 'running' FROM users WHERE email = 'admin@sny.test'`)
	admin.Post("/applications/export", url.Values{"query": {"q=bulk"}})
	var same int
	_ = app.Pool.QueryRow(context.Background(), `SELECT count(*) FROM exports WHERE query = 'q=bulk'`).Scan(&same)
	if same != 1 {
		t.Errorf("got %d jobs for the same filters, want 1 (reused)", same)
	}
}
