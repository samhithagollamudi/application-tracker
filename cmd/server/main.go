// Command server runs the SNY Cosmos application tracker.
//
//	server            start the HTTP server (runs migrations first)
//	server migrate    apply migrations and exit
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/config"
	"github.com/ChandraShekar2001/application-tracker/internal/db"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/httpx"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
	"github.com/ChandraShekar2001/application-tracker/internal/web/static"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve(ctx, log, cfg, pool)
	case "migrate":
		log.Info("migrations up to date")
		return nil
	default:
		return fmt.Errorf("unknown command %q (want: serve, migrate)", cmd)
	}
}

func serve(ctx context.Context, log *slog.Logger, cfg config.Config, pool *pgxpool.Pool) error {
	q := gen.New(pool)
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", static.Handler()))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(c); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		rows, err := q.ListBranches(r.Context())
		if err != nil {
			log.Error("list branches", "id", httpx.RequestID(r.Context()), "err", err)
			renderError(w, r, http.StatusInternalServerError)
			return
		}
		names := make([]string, len(rows))
		for i, b := range rows {
			names[i] = b.Name
		}
		_ = pages.Home(names).Render(r.Context(), w)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		renderError(w, r, http.StatusNotFound)
	})

	handler := httpx.Chain(mux,
		httpx.WithRequestID,
		httpx.Logger(log),
		httpx.Recover(log),
		httpx.SecurityHeaders(cfg.IsProd()),
	)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // CSV export streams; revisit in milestone 7
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

func renderError(w http.ResponseWriter, r *http.Request, status int) {
	title, msg := "Something went wrong", "Please try again. If it keeps happening, contact the admin."
	if status == http.StatusNotFound {
		title, msg = "Page not found", "The page you asked for doesn't exist, or you don't have access to it."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = pages.Error(status, title, msg).Render(r.Context(), w)
}
