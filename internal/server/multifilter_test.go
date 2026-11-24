package server_test

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestMultiValueFilters(t *testing.T) {
	w := newWorld(t)
	var chennai, hyd int16
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM branches WHERE name = 'Chennai'`).Scan(&chennai)
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM branches WHERE name = 'Hyderabad'`).Scan(&hyd)
	var alpha int64
	_ = w.app.Pool.QueryRow(t.Context(), `SELECT id FROM consultancies WHERE name = 'Alpha Overseas'`).Scan(&alpha)
	// Give Hyderabad's application a different status.
	w.hyd.Post(fmt.Sprintf("/applications/%d/status", w.aHyd), url.Values{"status": {"admitted"}})

	all := []string{"priya@example.com", "rahul@example.com", "kavya@example.com"}
	shows := func(body, email string) bool {
		return strings.Contains(body, `<div class="small muted">`+email+`</div>`) || // applications table
			strings.Contains(body, " · "+email+"</div>") // students table
	}
	check := func(path string, want ...string) {
		t.Helper()
		body := w.admin.Get(path).Body
		for _, e := range all {
			wantIt := false
			for _, x := range want {
				wantIt = wantIt || x == e
			}
			if got := shows(body, e); got != wantIt {
				t.Errorf("%s: shows %s = %v, want %v", path, e, got, wantIt)
			}
		}
	}

	check(fmt.Sprintf("/applications?branch=%d&branch=%d", chennai, hyd), "priya@example.com", "rahul@example.com")
	check("/applications?status=applied&status=admitted", "priya@example.com", "rahul@example.com")
	check("/applications?status=admitted", "rahul@example.com")
	check("/applications?source=internal&source=b2b", all...)
	check(fmt.Sprintf("/applications?consultancy=none&consultancy=%d", alpha), all...)
	check(fmt.Sprintf("/applications?consultancy=%d", alpha), "kavya@example.com")
	check("/applications?intake=autumn&intake=spring&year=2027", "priya@example.com")
	// Filters combine with AND across groups, OR within a group.
	check(fmt.Sprintf("/applications?branch=%d&branch=%d&status=admitted", chennai, hyd), "rahul@example.com")
	// Garbage values are ignored, not errors.
	check("/applications?status=bogus&branch=abc&intake=summer&year=xx", all...)

	check(fmt.Sprintf("/students?branch=%d&branch=%d", chennai, hyd), "priya@example.com", "rahul@example.com")
	check("/students?status=applied&status=admitted", "priya@example.com", "rahul@example.com")

	// The dropdown shows what's ticked.
	body := w.admin.Get(fmt.Sprintf("/applications?branch=%d&branch=%d", chennai, hyd)).Body
	if !strings.Contains(body, ">Chennai, Hyderabad</span>") {
		t.Error("multi-select summary doesn't list the ticked branches")
	}

	// CSV export honours several values too.
	r := w.admin.Post("/applications/export", url.Values{"query": {"status=applied&status=admitted"}})
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(r.Body, "\uFEFF"))).ReadAll()
	if err != nil || len(recs) != 3 {
		t.Fatalf("multi-status export: %d rows (%v), want header + 2", len(recs), err)
	}
}
