// Package demo fills an EMPTY development database with sample data
// (mirrors the clickable prototype) so the app can be tried end to end.
package demo

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/activity"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
)

var (
	branches     = []string{"Chennai", "Hyderabad", "Vijayawada", "Bengaluru", "Pune"}
	universities = [][2]string{{"Lund University", "Sweden"}, {"Uppsala University", "Sweden"}, {"Chalmers University of Technology", "Sweden"},
		{"Linköping University", "Sweden"}, {"Jönköping University", "Sweden"}, {"Aalto University", "Finland"}, {"University of Helsinki", "Finland"},
		{"LUT University", "Finland"}, {"University of Turku", "Finland"}, {"Aarhus University", "Denmark"}, {"University of Southern Denmark", "Denmark"},
		{"Politecnico di Milano", "Italy"}, {"University of Padova", "Italy"}, {"Riga Technical University", "Latvia"}, {"NEOMA Business School", "France"}}
	consultancies = []string{"Alpha Overseas", "Bright Path Education", "Crest Study Abroad", "Delta Global Careers"}
	firstNames    = []string{"Aarav", "Diya", "Rohan", "Ananya", "Karthik", "Meera", "Vikram", "Sneha", "Arjun", "Priya", "Rahul", "Kavya", "Sai", "Nisha", "Aditya",
		"Pooja", "Harsha", "Lakshmi", "Varun", "Divya", "Nikhil", "Swathi", "Manoj", "Keerthi", "Siddharth", "Bhavana", "Ravi", "Shreya", "Tarun", "Ishita"}
	lastNames  = []string{"Reddy", "Iyer", "Sharma", "Nair", "Rao", "Kumar", "Patel", "Menon", "Kulkarni", "Naidu", "Pillai", "Deshpande", "Varma", "Joshi", "Krishnan"}
	programmes = []string{"Computer Science", "Data Science", "Embedded Systems", "Industrial Management", "Biotechnology", "Sustainable Energy",
		"Machine Learning", "Business Analytics", "Mechanical Engineering", "Software Engineering"}
	bachelors = []string{"Anna University", "JNTU Hyderabad", "Osmania University", "VIT Vellore", "SRM University", "Andhra University", "VTU Belagavi"}
	statuses  = []string{"applied", "applied", "applied", "admitted", "admitted", "reserve", "rejected", "withdrawn", "fees_paid", "visa_granted", "visa_rejected"}
)

type User struct{ Email, Role string }

// Seed inserts the demo data and returns the users created and their shared password.
func Seed(ctx context.Context, pool *pgxpool.Pool, domain string) ([]User, string, error) {
	var n int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users) + (SELECT count(*) FROM students)`).Scan(&n); err != nil {
		return nil, "", err
	}
	if n > 0 {
		return nil, "", errors.New("database already has users or students; seed-demo only runs on an empty database")
	}
	pw := auth.GeneratePassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return nil, "", err
	}
	rng := rand.New(rand.NewPCG(7, 42))
	pick := func(xs []string) string { return xs[rng.IntN(len(xs))] }
	var users []User

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		addUser := func(name, email, role, branch string) (int64, error) {
			var id int64
			users = append(users, User{email, role})
			return id, tx.QueryRow(ctx, `INSERT INTO users (name, email, role, branch_id, password_hash)
				VALUES ($1, $2, $3, (SELECT id FROM branches WHERE name = NULLIF($4, '')), $5) RETURNING id`,
				name, email, role, branch, hash).Scan(&id)
		}
		if _, err := addUser("Admin", "admin@"+domain, "admin", ""); err != nil {
			return err
		}
		agent, err := addUser("Agent network", "agentnetwork@"+domain, "agent_network", "")
		if err != nil {
			return err
		}
		consultant := map[string]int64{}
		for _, b := range branches {
			id, err := addUser("Consultant – "+b, "consultant."+strings.ToLower(b)+"@"+domain, "consultant", b)
			if err != nil {
				return err
			}
			consultant[b] = id
		}
		var uniIDs []int64
		for _, u := range universities {
			var id int64
			if err := tx.QueryRow(ctx, `INSERT INTO universities (name, country) VALUES ($1, $2) RETURNING id`, u[0], u[1]).Scan(&id); err != nil {
				return err
			}
			uniIDs = append(uniIDs, id)
		}
		var consIDs []int64
		for _, c := range consultancies {
			var id int64
			if err := tx.QueryRow(ctx, `INSERT INTO consultancies (name) VALUES ($1) RETURNING id`, c).Scan(&id); err != nil {
				return err
			}
			consIDs = append(consIDs, id)
		}

		k := 0
		for bi, b := range branches {
			for j := 0; j < 6; j++ {
				b2b := j >= 4
				first, last := firstNames[(bi*6+j)%len(firstNames)], pick(lastNames)
				created := time.Now().Add(-time.Duration(3+rng.IntN(70)) * 24 * time.Hour)
				owner, source, branch := consultant[b], "internal", &b
				var cons *int64
				if b2b {
					owner, source, branch = agent, "b2b", nil
					c := consIDs[rng.IntN(len(consIDs))]
					cons = &c
				}
				k++
				var sid int64
				if err := tx.QueryRow(ctx, `INSERT INTO students (source, branch_id, consultancy_id, owner_id, first_name, family_name,
					gender, date_of_birth, email, address, ba_university, ba_specialisation, ba_duration, ba_grade, xii_board, xii_subjects, xii_grade,
					created_at, updated_at)
					VALUES ($1, (SELECT id FROM branches WHERE name = $2), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, '4 years', $13, $14, 'MPC', $15, $16, $16)
					RETURNING id`,
					source, branch, cons, owner, first, last, pick([]string{"Female", "Male"}),
					fmt.Sprintf("%d-%02d-%02d", 1998+rng.IntN(5), 1+rng.IntN(12), 1+rng.IntN(28)),
					fmt.Sprintf("%s.%s%d@example.com", strings.ToLower(first), strings.ToLower(last), k),
					"[Address as per passport], "+b, pick(bachelors), pick([]string{"Computer Science", "Electronics", "Mechanical", "Commerce"}),
					fmt.Sprintf("%.2f", 7+rng.Float64()*2.5), pick([]string{"CBSE", "State Board", "ICSE"}), fmt.Sprintf("%d%%", 75+rng.IntN(22)),
					created).Scan(&sid); err != nil {
					return err
				}
				if err := activity.Log(ctx, tx, sid, owner, activity.StudentCreated, "Student created"); err != nil {
					return err
				}
				used := map[int]bool{}
				for a := 0; a < 1+rng.IntN(3); a++ {
					ui := rng.IntN(len(uniIDs))
					for used[ui] {
						ui = rng.IntN(len(uniIDs))
					}
					used[ui] = true
					st := pick(statuses)
					admitted := st == "admitted" || st == "fees_paid" || st == "visa_granted" || st == "visa_rejected"
					var fee, cur, per *string
					if admitted {
						f, c, p := fmt.Sprint((10+rng.IntN(8))*1000), "EUR", "year"
						if universities[ui][1] == "Sweden" {
							f, c = fmt.Sprint((10+rng.IntN(8))*10000), "SEK"
						}
						fee, cur, per = &f, &c, &p
					}
					if _, err := tx.Exec(ctx, `INSERT INTO applications (student_id, university_id, programme_level, programme, priority,
						intake, year, application_number, submission_date, status, tuition_fee, tuition_currency, tuition_per, created_by,
						created_at, updated_at)
						VALUES ($1, $2, 'masters', $3, $4, $5, 2027, $6, $7::date, $8, $9, $10, $11, $12, $13, $13)`,
						sid, uniIDs[ui], "MSc "+pick(programmes), a+1, pick([]string{"autumn", "spring"}),
						fmt.Sprint(400000+rng.IntN(99999)), created.Format("2006-01-02"), st, fee, cur, per, owner, created); err != nil {
						return err
					}
					if err := activity.Log(ctx, tx, sid, owner, activity.ApplicationAdded, "Application added: "+universities[ui][0]); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return users, pw, err
}
