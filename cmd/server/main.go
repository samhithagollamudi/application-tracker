// Command server runs the SNY Cosmos application tracker.
//
//	server                                         migrate, then serve HTTP
//	server migrate                                 apply migrations and exit
//	server create-admin --email E --name "N"       create an admin; prints a generated password once
//	server seed-demo [--domain demo.local]         development only: sample data in an EMPTY database
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // distroless has no zoneinfo; dates display in IST

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/config"
	"github.com/ChandraShekar2001/application-tracker/internal/db"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/demo"
	"github.com/ChandraShekar2001/application-tracker/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, log); err != nil {
		return err
	}

	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(ctx, log, cfg, pool)
	case "migrate":
		log.Info("migrations up to date")
		return nil
	case "create-admin":
		return createAdmin(ctx, pool, args)
	case "seed-demo":
		if cfg.IsProd() {
			return errors.New("seed-demo is refused when APP_ENV=production")
		}
		fs := flag.NewFlagSet("seed-demo", flag.ContinueOnError)
		domain := fs.String("domain", "demo.local", "email domain for the demo users")
		if err := fs.Parse(args); err != nil {
			return err
		}
		users, pw, err := demo.Seed(ctx, pool, *domain)
		if err != nil {
			return err
		}
		fmt.Println("\nDemo data created. Every demo user has the same password:")
		fmt.Printf("  Password: %s\n\n", pw)
		for _, u := range users {
			fmt.Printf("  %-14s %s\n", u.Role, u.Email)
		}
		fmt.Println()
		return nil
	default:
		return fmt.Errorf("unknown command %q (want: serve, migrate, create-admin, seed-demo)", cmd)
	}
}

func createAdmin(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	email := fs.String("email", "", "admin user ID (email)")
	name := fs.String("name", "Admin", "display name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	e := strings.TrimSpace(*email)
	if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
		return errors.New("--email must be a plain email address")
	}
	pw := auth.GeneratePassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := gen.New(pool).CreateUser(ctx, gen.CreateUserParams{
		Name: strings.TrimSpace(*name), Email: e, Role: "admin", PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	fmt.Printf("\nAdmin created.\n  User ID:  %s\n  Password: %s\n\nCopy the password now; it is not stored anywhere in plain text.\n\n", e, pw)
	return nil
}

func serve(ctx context.Context, log *slog.Logger, cfg config.Config, pool *pgxpool.Pool) error {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           server.New(server.Deps{Cfg: cfg, Pool: pool, Log: log}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute, // CSV export streams large files
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}
