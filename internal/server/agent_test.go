package server_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAgentNetworkConsultancyOther(t *testing.T) {
	w := newWorld(t)

	// The new-student form shows the consultancy dropdown to the agent network only.
	if r := w.agent.Get("/students/new"); !strings.Contains(r.Body, "Referred by") || !strings.Contains(r.Body, "Alpha Overseas") {
		t.Fatal("agent network form lacks the consultancy dropdown")
	}
	if r := w.chennai.Get("/students/new"); strings.Contains(r.Body, "Referred by") {
		t.Fatal("consultant form shows the consultancy dropdown")
	}

	// "Other" with a new name adds it to the admin list, marked as added by the agent.
	sid := studentID(t, w.agent.Post("/students/new", url.Values{
		"email": {"b2b-new@example.com"}, "consultancy_id": {"__other"}, "consultancy_other": {"  Bright   Path Education "},
	}))
	if n := w.count(t, `SELECT count(*) FROM consultancies c JOIN users u ON u.id = c.created_by
		WHERE c.name = 'Bright Path Education' AND u.role = 'agent_network'`); n != 1 {
		t.Fatal("'Other' consultancy not added to the list")
	}
	if n := w.count(t, `SELECT count(*) FROM students s JOIN consultancies c ON c.id = s.consultancy_id
		WHERE s.id = $1 AND c.name = 'Bright Path Education'`, sid); n != 1 {
		t.Fatal("student not linked to the new consultancy")
	}

	// Typing an existing name (any case) reuses it instead of duplicating.
	studentID(t, w.agent.Post("/students/new", url.Values{
		"email": {"b2b-two@example.com"}, "consultancy_id": {"__other"}, "consultancy_other": {"alpha overseas"},
	}))
	if n := w.count(t, `SELECT count(*) FROM consultancies WHERE lower(name) = 'alpha overseas'`); n != 1 {
		t.Fatal("existing consultancy duplicated")
	}

	// Unknown consultancy id is a form error.
	r := w.agent.Post("/students/new", url.Values{"email": {"b2b-bad@example.com"}, "consultancy_id": {"999999"}})
	if r.Status != http.StatusUnprocessableEntity || !strings.Contains(r.Body, "Pick a consultancy") {
		t.Fatalf("unknown consultancy: %d", r.Status)
	}

	// The agent network can change the consultancy when editing; the list filter finds it.
	r = w.agent.Post(fmt.Sprintf("/students/%d/edit", w.sB2B), url.Values{
		"email": {"kavya@example.com"}, "first_name": {"Kavya"}, "consultancy_id": {"__other"}, "consultancy_other": {"Crest Study Abroad"},
	})
	if r.Status != http.StatusSeeOther {
		t.Fatalf("edit consultancy: %d\n%s", r.Status, r.Body)
	}
	var crest int64
	if err := w.app.Pool.QueryRow(t.Context(), `SELECT id FROM consultancies WHERE name = 'Crest Study Abroad'`).Scan(&crest); err != nil {
		t.Fatal(err)
	}
	list := w.agent.Get(fmt.Sprintf("/students?consultancy=%d", crest))
	if !strings.Contains(list.Body, " · kavya@example.com</div>") || strings.Contains(list.Body, " · b2b-new@example.com</div>") {
		t.Fatal("consultancy filter wrong")
	}
}

func TestAgentNetworkSeesAllB2BOnly(t *testing.T) {
	w := newWorld(t)
	// A second agent-network user sees the same B2B students (not per-owner).
	w.app.CreateUser("Agent two", "agent2@sny.test", "agent_network", "")
	a2 := w.app.Login("agent2@sny.test")
	if r := a2.Get(fmt.Sprintf("/students/%d", w.sB2B)); r.Status != http.StatusOK {
		t.Fatalf("second agent can't see B2B student: %d", r.Status)
	}
	if r := a2.Get(fmt.Sprintf("/students/%d", w.sChennai)); r.Status != http.StatusNotFound {
		t.Fatalf("agent sees internal student: %d", r.Status)
	}
}
