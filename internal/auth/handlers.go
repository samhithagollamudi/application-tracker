package auth

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

const badCredentials = "That user ID and password don’t match."

func (m *Manager) LoginPage(w http.ResponseWriter, r *http.Request) {
	if reqctx.User(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_ = pages.Login("", "").Render(r.Context(), w)
}

func (m *Manager) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := strings.TrimSpace(r.PostFormValue("email"))
	pw := r.PostFormValue("password")
	ipKey := "ip:" + clientIP(r, m.trustProxy)
	emailKey := "email:" + strings.ToLower(email)

	fail := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = pages.Login(email, msg).Render(ctx, w)
	}

	for _, k := range []string{ipKey, emailKey} {
		if blocked, wait := m.emailOrIP(k).Blocked(k); blocked {
			mins := int(math.Ceil(wait.Minutes()))
			fail(fmt.Sprintf("Too many failed attempts. Try again in %d minute%s.", mins, plural(mins)))
			return
		}
	}
	if email == "" || pw == "" || len(email) > 254 || len(pw) > 256 {
		fail(badCredentials)
		return
	}

	row, err := m.q.GetUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		m.log.Error("login lookup", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	hash := dummyHash
	if err == nil {
		hash = row.PasswordHash
	}
	ok, herr := CheckPassword(hash, pw)
	if herr != nil {
		m.log.Error("check password", "err", herr)
	}
	if err != nil || !ok {
		m.ipLimit.Fail(ipKey)
		m.emailLimit.Fail(emailKey)
		fail(badCredentials)
		return
	}
	if !row.Active {
		fail("This account is deactivated. Ask your admin.")
		return
	}

	m.emailLimit.Reset(emailKey)
	if err := m.Sessions.RenewToken(ctx); err != nil {
		m.log.Error("renew session", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	m.Sessions.Put(ctx, keyUserID, row.ID)
	m.Sessions.Put(ctx, keyAuthVersion, int(row.AuthVersion))
	m.Sessions.Put(ctx, keyCSRF, newToken())
	m.log.Info("login", "user_id", row.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if err := m.Sessions.Destroy(r.Context()); err != nil {
		m.log.Error("destroy session", "err", err)
	}
	Redirect(w, r, "/login")
}

func (m *Manager) emailOrIP(key string) *limiter {
	if strings.HasPrefix(key, "ip:") {
		return m.ipLimit
	}
	return m.emailLimit
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
