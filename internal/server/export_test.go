package server_test

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCSVExport(t *testing.T) {
	w := newWorld(t)
	// A hostile cell value must not become a formula.
	w.chennai.Post(fmt.Sprintf("/students/%d/edit", w.sChennai), url.Values{
		"email": {"priya@example.com"}, "first_name": {"=HYPERLINK(\"http://evil\")"}, "family_name": {"Rao"},
	})

	r := w.admin.Post("/applications/export", url.Values{"query": {""}})
	if r.Status != http.StatusOK || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("export: %d %s", r.Status, r.Header.Get("Content-Type"))
	}
	if !strings.Contains(r.Header.Get("Content-Disposition"), "attachment; filename=\"applications-") {
		t.Error("missing attachment filename")
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(r.Body, "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 4 { // header + 3 applications
		t.Fatalf("got %d rows, want 4", len(recs))
	}
	if recs[0][0] != "Application ID" || recs[0][len(recs[0])-1] != "Last updated" {
		t.Errorf("unexpected header %v", recs[0])
	}
	for _, rec := range recs {
		if len(rec) != len(recs[0]) {
			t.Fatalf("row has %d columns, header has %d", len(rec), len(recs[0]))
		}
	}
	col := func(name string) int {
		for i, h := range recs[0] {
			if h == name {
				return i
			}
		}
		t.Fatalf("no column %q", name)
		return -1
	}
	found := false
	for _, rec := range recs[1:] {
		if rec[col("Application email")] == "priya@example.com" {
			found = true
			if !strings.HasPrefix(rec[col("First name")], "'=") {
				t.Errorf("formula not neutralised: %q", rec[col("First name")])
			}
			if rec[col("Branch")] != "Chennai" || rec[col("University")] != "Lund University" || rec[col("Status")] != "Applied" {
				t.Errorf("wrong values: %v", rec)
			}
		}
	}
	if !found {
		t.Fatal("priya's application missing")
	}

	// Filters apply to the export exactly as on screen.
	r = w.admin.Post("/applications/export", url.Values{"query": {"source=b2b"}})
	recs, _ = csv.NewReader(strings.NewReader(strings.TrimPrefix(r.Body, "\uFEFF"))).ReadAll()
	if len(recs) != 2 || recs[1][col("Application email")] != "kavya@example.com" {
		t.Fatalf("filtered export wrong: %v", recs)
	}

	// Only admins can export.
	if r := w.chennai.Post("/applications/export", nil); r.Status != http.StatusNotFound {
		t.Fatalf("consultant export: %d", r.Status)
	}
	if r := w.agent.Post("/applications/export", nil); r.Status != http.StatusNotFound {
		t.Fatalf("agent export: %d", r.Status)
	}
}
