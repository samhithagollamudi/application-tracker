package db_test

import (
	"context"
	"testing"

	"github.com/ChandraShekar2001/application-tracker/internal/db"
	"github.com/ChandraShekar2001/application-tracker/internal/db/dbtest"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
)

func TestMigrationsSeedBranches(t *testing.T) {
	pool := dbtest.New(t)
	got, err := gen.New(pool).ListBranches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Chennai", "Hyderabad", "Vijayawada", "Bengaluru", "Pune"}
	if len(got) != len(want) {
		t.Fatalf("got %d branches, want %d", len(got), len(want))
	}
	for i, b := range got {
		if b.Name != want[i] {
			t.Errorf("branch %d = %q, want %q", i, b.Name, want[i])
		}
	}
}

// Every migration must be reversible: up, all the way down, and up again.
func TestMigrationsReversible(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, dbtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.MigrateRoundTrip(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestStudentEmailUniqueIgnoresCaseAndSpaces(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	var uid int64
	err := pool.QueryRow(ctx, `INSERT INTO users (name, email, role, password_hash)
		VALUES ('A', 'a@x.com', 'agent_network', 'x') RETURNING id`).Scan(&uid)
	if err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO students (source, owner_id, email) VALUES ('b2b', $1, $2)`
	if _, err := pool.Exec(ctx, ins, uid, "Priya@Example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ins, uid, "  priya@example.com "); err == nil {
		t.Fatal("expected unique violation for same email with different case/whitespace")
	}
}
