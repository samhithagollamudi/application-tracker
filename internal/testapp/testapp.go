// Package testapp runs the full application against a fresh database and
// provides signed-in HTTP clients for each role.
package testapp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/config"
	"github.com/ChandraShekar2001/application-tracker/internal/db/dbtest"
	"github.com/ChandraShekar2001/application-tracker/internal/server"
)

type App struct {
	T    testing.TB
	Pool *pgxpool.Pool
	Srv  *httptest.Server
	hash string // hash of Password, computed once (argon2 is deliberately slow)
}

// Password is the password of every user created with CreateUser.
const Password = "correct-horse-battery"

// Option tweaks the server under test.
type Option func(*server.Deps)

// WithExportDirectLimit makes exports above n rows run in the background.
func WithExportDirectLimit(n int) Option { return func(d *server.Deps) { d.ExportDirectLimit = n } }

func New(t testing.TB, opts ...Option) *App {
	t.Helper()
	pool := dbtest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := server.Deps{
		Cfg:  config.Config{Env: "development"},
		Pool: pool,
		Log:  log,
		Ctx:  t.Context(),
	}
	for _, o := range opts {
		o(&deps)
	}
	srv := httptest.NewServer(server.New(deps))
	t.Cleanup(srv.Close)
	h, err := auth.HashPassword(Password)
	if err != nil {
		t.Fatal(err)
	}
	return &App{T: t, Pool: pool, Srv: srv, hash: h}
}

// CreateUser inserts a user. branch is a branch name for consultants, "" otherwise.
func (a *App) CreateUser(name, email, role, branch string) int64 {
	a.T.Helper()
	var id int64
	err := a.Pool.QueryRow(context.Background(), `
		INSERT INTO users (name, email, role, branch_id, password_hash)
		VALUES ($1, $2, $3, (SELECT id FROM branches WHERE name = NULLIF($4, '')), $5)
		RETURNING id`, name, email, role, branch, a.hash).Scan(&id)
	if err != nil {
		a.T.Fatalf("create user %s: %v", email, err)
	}
	return id
}

// Exec runs SQL directly (for test setup).
func (a *App) Exec(sql string, args ...any) {
	a.T.Helper()
	if _, err := a.Pool.Exec(context.Background(), sql, args...); err != nil {
		a.T.Fatalf("exec: %v", err)
	}
}

// Client is a browser-like client with cookies that does not follow redirects.
type Client struct {
	app  *App
	HTTP *http.Client
	csrf string
}

func (a *App) Anon() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{app: a, HTTP: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Login returns a client signed in as email (password = Password).
func (a *App) Login(email string) *Client {
	a.T.Helper()
	c := a.Anon()
	res := c.Post("/login", url.Values{"email": {email}, "password": {Password}})
	if res.Status != http.StatusSeeOther {
		a.T.Fatalf("login %s: status %d", email, res.Status)
	}
	c.Get("/students") // refresh the CSRF token (rotated at login)
	return c
}

type Response struct {
	Status   int
	Body     string
	Location string
	Header   http.Header
}

var csrfRe = regexp.MustCompile(`<meta name="csrf-token" content="([^"]+)"`)

func (c *Client) do(req *http.Request) Response {
	c.app.T.Helper()
	res, err := c.HTTP.Do(req)
	if err != nil {
		c.app.T.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	body := string(b)
	if m := csrfRe.FindStringSubmatch(body); m != nil {
		c.csrf = m[1]
	}
	return Response{Status: res.StatusCode, Body: body, Location: res.Header.Get("Location"), Header: res.Header}
}

func (c *Client) Get(path string) Response {
	c.app.T.Helper()
	req, _ := http.NewRequest(http.MethodGet, c.app.Srv.URL+path, nil)
	return c.do(req)
}

// Post submits a form, adding the CSRF token (fetching /login first if none is known yet).
func (c *Client) Post(path string, form url.Values) Response {
	c.app.T.Helper()
	if c.csrf == "" {
		c.Get("/login")
	}
	if form == nil {
		form = url.Values{}
	}
	if !form.Has(auth.CSRFField) {
		form.Set(auth.CSRFField, c.csrf)
	}
	return c.PostRaw(path, form)
}

// PostRaw submits a form exactly as given (no CSRF token added).
func (c *Client) PostRaw(path string, form url.Values) Response {
	c.app.T.Helper()
	req, _ := http.NewRequest(http.MethodPost, c.app.Srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}
