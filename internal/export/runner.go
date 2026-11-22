package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
)

// Runner executes background export jobs stored in the exports table.
//
// Each job reads all its rows from ONE repeatable-read snapshot, so the file
// is consistent even while staff keep editing (no rows duplicated or skipped
// between batches). It is written gzip-compressed into the database and
// deleted after 24 hours. Jobs survive restarts: anything left "running" by
// a previous process is queued again on startup.
type Runner struct {
	pool    *pgxpool.Pool
	log     *slog.Logger
	queue   chan int64
	workers int
	started sync.Once
}

func NewRunner(pool *pgxpool.Pool, log *slog.Logger) *Runner {
	return &Runner{pool: pool, log: log, queue: make(chan int64, 100), workers: 2}
}

// Start launches the workers and housekeeping; they stop when ctx ends.
func (r *Runner) Start(ctx context.Context) {
	r.started.Do(func() {
		q := gen.New(r.pool)
		if err := q.RequeueInterruptedExports(ctx); err != nil {
			r.log.Error("requeue exports", "err", err)
		}
		for i := 0; i < r.workers; i++ {
			go r.worker(ctx)
		}
		go r.housekeeping(ctx)
	})
}

// Enqueue asks a worker to run job id. If the queue is full the job stays
// "queued" in the database and the housekeeping sweep picks it up.
func (r *Runner) Enqueue(id int64) {
	select {
	case r.queue <- id:
	default:
	}
}

func (r *Runner) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-r.queue:
			r.run(ctx, id)
		}
	}
}

func (r *Runner) housekeeping(ctx context.Context) {
	sweep := func() {
		q := gen.New(r.pool)
		if n, err := q.DeleteExpiredExports(ctx); err == nil && n > 0 {
			r.log.Info("expired exports deleted", "count", n)
		}
		ids, err := q.ListQueuedExports(ctx)
		if err != nil {
			return
		}
		for _, id := range ids {
			r.Enqueue(id)
		}
	}
	sweep()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

const failMsg = "The export failed. Please try again."

func (r *Runner) run(ctx context.Context, id int64) {
	q := gen.New(r.pool)
	job, err := q.ClaimExport(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return // already taken by another worker, or no longer queued
	}
	if err != nil {
		r.log.Error("claim export", "export_id", id, "err", err)
		return
	}
	start := time.Now()

	fail := func(msg string, err error) {
		if ctx.Err() != nil {
			return // shutting down: stays "running" and is requeued on next start
		}
		r.log.Error("export failed", "export_id", id, "err", err)
		if ferr := q.FailExport(context.WithoutCancel(ctx), gen.FailExportParams{ID: id, Error: &msg}); ferr != nil {
			r.log.Error("mark export failed", "export_id", id, "err", ferr)
		}
	}

	row, err := q.GetUserByID(ctx, job.UserID)
	if err != nil {
		fail(failMsg, err)
		return
	}
	if !row.Active || model.Role(row.Role) != model.RoleAdmin {
		fail("Your account can no longer export.", errors.New("user not an active admin"))
		return
	}
	u := &model.User{ID: row.ID, Role: model.Role(row.Role), BranchID: row.BranchID}
	vals, _ := url.ParseQuery(job.Query)
	f := applications.ParseFilter(vals)

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		fail(failMsg, err)
		return
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	scope := access.ScopeOf(u)
	total, err := applications.Count(ctx, tx, scope, f)
	if err != nil {
		fail(failMsg, err)
		return
	}
	_ = q.SetExportProgress(ctx, gen.SetExportProgressParams{ID: id, TotalRows: int32(total), DoneRows: 0})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	cw := &countingWriter{w: gz}
	n, err := Write(ctx, tx, scope, f, cw, nil, func(n int) {
		if n%2000 == 0 {
			_ = q.SetExportProgress(ctx, gen.SetExportProgressParams{ID: id, TotalRows: int32(total), DoneRows: int32(n)})
		}
	})
	if err == nil {
		err = gz.Close()
	}
	if err != nil {
		fail(failMsg, err)
		return
	}
	_ = tx.Commit(ctx)

	name := FileName(time.Now())
	size := cw.n
	if err := q.FinishExport(ctx, gen.FinishExportParams{
		ID: id, DoneRows: int32(n), File: buf.Bytes(), FileName: &name, FileSize: &size,
	}); err != nil {
		fail(failMsg, err)
		return
	}
	r.log.Info("csv export ready", "export_id", id, "user_id", u.ID, "rows", n,
		"bytes", size, "gzip_bytes", buf.Len(), "dur_ms", time.Since(start).Milliseconds())
}

type countingWriter struct {
	w interface{ Write([]byte) (int, error) }
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
