package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (w *world) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := w.app.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStudentCreatedWithFirstApplication(t *testing.T) {
	w := newWorld(t)
	r := w.chennai.Get(fmt.Sprintf("/students/%d", w.sChennai))
	for _, want := range []string{"Priya Rao", "Lund University", "Applied", "Chennai branch", "Student created", "Application added: Lund University (Applied)"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
	// Internal student belongs to the creator's branch; B2B student has the consultancy and no branch.
	if n := w.count(t, `SELECT count(*) FROM students s JOIN branches b ON b.id = s.branch_id
		WHERE s.id = $1 AND s.source = 'internal' AND b.name = 'Chennai'`, w.sChennai); n != 1 {
		t.Error("chennai student not internal/Chennai")
	}
	if n := w.count(t, `SELECT count(*) FROM students s JOIN consultancies c ON c.id = s.consultancy_id
		WHERE s.id = $1 AND s.source = 'b2b' AND s.branch_id IS NULL AND c.name = 'Alpha Overseas'`, w.sB2B); n != 1 {
		t.Error("agent student not b2b with consultancy")
	}
}

func TestEmptyFirstApplicationIsSkipped(t *testing.T) {
	w := newWorld(t)
	sid := studentID(t, w.chennai.Post("/students/new", url.Values{"email": {"solo@example.com"}, "tuition_currency": {"EUR"}, "year": {""}}))
	if n := w.count(t, `SELECT count(*) FROM applications WHERE student_id = $1`, sid); n != 0 {
		t.Fatalf("got %d applications, want 0", n)
	}
	// Partly filled application without a university is an error, not silently dropped.
	r := w.chennai.Post("/students/new", url.Values{"email": {"half@example.com"}, "programme": {"MSc Data"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "Pick a university") {
		t.Fatalf("partial application: %d", r.Status)
	}
	if n := w.count(t, `SELECT count(*) FROM students WHERE email = 'half@example.com'`); n != 0 {
		t.Fatal("student saved even though the application was invalid")
	}
}

func TestOneOffUniversityIsNotAddedToList(t *testing.T) {
	w := newWorld(t)
	before := w.count(t, `SELECT count(*) FROM universities`)
	r := w.chennai.Post(fmt.Sprintf("/students/%d/applications/new", w.sChennai), url.Values{
		"uni_other": {"Tiny Polytechnic"}, "uni_other_country": {"Estonia"}, "programme_level": {"masters"},
		"tuition_fee": {"9,500.50"}, "tuition_currency": {"EUR"}, "tuition_per": {"year"},
	})
	if r.Status != http.StatusSeeOther {
		t.Fatalf("add one-off: %d\n%s", r.Status, r.Body)
	}
	if n := w.count(t, `SELECT count(*) FROM universities`); n != before {
		t.Fatal("one-off university was added to the list")
	}
	if n := w.count(t, `SELECT count(*) FROM applications WHERE custom_university_name = 'Tiny Polytechnic'
		AND custom_university_country = 'Estonia' AND tuition_fee = 9500.50 AND tuition_currency = 'EUR'`); n != 1 {
		t.Fatal("one-off application not stored as expected")
	}
	page := w.chennai.Get(fmt.Sprintf("/students/%d", w.sChennai))
	if !strings.Contains(page.Body, "One-off") || !strings.Contains(page.Body, "EUR 9500.50 per year") {
		t.Error("detail page doesn't show the one-off application")
	}

	// Typing an existing list name under "Other" uses the list entry.
	w.chennai.Post(fmt.Sprintf("/students/%d/applications/new", w.sChennai), url.Values{"uni_other": {"aalto university"}})
	if n := w.count(t, `SELECT count(*) FROM applications a JOIN universities u ON u.id = a.university_id
		WHERE a.student_id = $1 AND u.name = 'Aalto University' AND a.custom_university_name IS NULL`, w.sChennai); n != 1 {
		t.Fatal("'Other' matching a list university should use the list entry")
	}
}

func TestInlineStatusChangeAndDelete(t *testing.T) {
	w := newWorld(t)
	path := fmt.Sprintf("/applications/%d/status", w.aChennai)
	req, _ := http.NewRequest(http.MethodPost, w.app.Srv.URL+path, strings.NewReader("status=admitted"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	// htmx sends the CSRF token as a header.
	page := w.chennai.Get(fmt.Sprintf("/students/%d", w.sChennai))
	tok := page.Body[strings.Index(page.Body, `name="csrf-token" content="`)+len(`name="csrf-token" content="`):]
	req.Header.Set("X-CSRF-Token", tok[:strings.IndexByte(tok, '"')])
	res, err := w.chennai.HTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("htmx status change: %d", res.StatusCode)
	}
	if n := w.count(t, `SELECT count(*) FROM applications WHERE id = $1 AND status = 'admitted'`, w.aChennai); n != 1 {
		t.Fatal("status not saved")
	}
	if n := w.count(t, `SELECT count(*) FROM activity_log WHERE student_id = $1 AND detail->>'text' LIKE 'Status Applied → Admitted%'`, w.sChennai); n != 1 {
		t.Fatal("status change not in activity log")
	}

	r := w.chennai.Post(fmt.Sprintf("/applications/%d/delete", w.aChennai), nil)
	if r.Status != http.StatusSeeOther {
		t.Fatalf("delete: %d", r.Status)
	}
	if n := w.count(t, `SELECT count(*) FROM applications WHERE id = $1`, w.aChennai); n != 0 {
		t.Fatal("application not deleted")
	}
}

func TestEditApplicationValidation(t *testing.T) {
	w := newWorld(t)
	r := w.hyd.Post(fmt.Sprintf("/applications/%d/edit", w.aHyd), url.Values{
		"university_id": {fmt.Sprint(w.uniLund)}, "priority": {"abc"}, "tuition_fee": {"lots"}, "status": {"bogus"},
	})
	if r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid edit: %d", r.Status)
	}
	for _, want := range []string{"whole number", "Enter an amount", "Pick a status"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("missing error %q", want)
		}
	}
}

func TestAdminMovesStudent(t *testing.T) {
	w := newWorld(t)
	var hydID int16
	_ = w.app.Pool.QueryRow(context.Background(), `SELECT id FROM branches WHERE name = 'Hyderabad'`).Scan(&hydID)
	r := w.admin.Post(fmt.Sprintf("/students/%d/move", w.sChennai), url.Values{"branch_id": {fmt.Sprint(hydID)}})
	if r.Status != http.StatusSeeOther {
		t.Fatalf("move: %d", r.Status)
	}
	// Now Hyderabad sees it (owner = Hyderabad consultant) and Chennai does not.
	if r := w.hyd.Get(fmt.Sprintf("/students/%d", w.sChennai)); r.Status != http.StatusOK || !strings.Contains(r.Body, "Owner: Hyderabad consultant") {
		t.Fatalf("hyderabad after move: %d", r.Status)
	}
	if r := w.chennai.Get(fmt.Sprintf("/students/%d", w.sChennai)); r.Status != http.StatusNotFound {
		t.Fatalf("chennai after move: %d", r.Status)
	}
}
