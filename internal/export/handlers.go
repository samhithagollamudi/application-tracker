package export

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/access"
	"github.com/ChandraShekar2001/application-tracker/internal/applications"
	"github.com/ChandraShekar2001/application-tracker/internal/auth"
	"github.com/ChandraShekar2001/application-tracker/internal/db/gen"
	"github.com/ChandraShekar2001/application-tracker/internal/httpx"
	"github.com/ChandraShekar2001/application-tracker/internal/model"
	"github.com/ChandraShekar2001/application-tracker/internal/reqctx"
	"github.com/ChandraShekar2001/application-tracker/internal/web"
	"github.com/ChandraShekar2001/application-tracker/internal/web/pages"
)

type Handler struct {
	Pool        *pgxpool.Pool
	Log         *slog.Logger
	Auth        *auth.Manager
	Runner      *Runner
	DirectLimit int // exports up to this many rows stream immediately
}

func (h *Handler) limit() int {
	if h.DirectLimit > 0 {
		return h.DirectLimit
	}
	return DefaultDirectLimit
}

// Start handles the Export button. The form carries the admin's current
// filter query. Small result sets download immediately; large ones become a
// background job and the admin is sent back to the list with a notice.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := reqctx.User(ctx)
	vals, _ := url.ParseQuery(r.PostFormValue("query"))
	query := model.QueryWithout(vals, "page")
	f := applications.ParseFilter(vals)
	scope := access.ScopeOf(u)

	total, err := applications.Count(ctx, h.Pool, scope, f)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	back := "/applications"
	if query != "" {
		back += "?" + query
	}

	if total <= h.limit() {
		h.stream(w, r, scope, f)
		return
	}

	q := gen.New(h.Pool)
	id, err := q.FindPendingExport(ctx, gen.FindPendingExportParams{UserID: u.ID, Query: query})
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = q.CreateExport(ctx, gen.CreateExportParams{UserID: u.ID, Query: query, TotalRows: int32(total)})
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	h.Runner.Enqueue(id)
	h.Log.Info("csv export queued", "id", httpx.RequestID(ctx), "export_id", id, "user_id", u.ID, "rows", total)
	h.Auth.Flash(ctx, fmt.Sprintf("Preparing your export of %s applications. Keep working — a notice with the download link appears when it’s ready (also under Exports).", model.Thousands(total)))
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request, scope access.Scope, f applications.Filter) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+FileName(time.Now())+`"`)
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	n, err := Write(ctx, h.Pool, scope, f, w, func() { _ = rc.Flush() }, nil)
	if err != nil {
		// Headers are already sent: kill the connection so a truncated file
		// can't be mistaken for a complete one.
		h.Log.Error("csv export failed", "id", httpx.RequestID(ctx), "err", err)
		panic(http.ErrAbortHandler)
	}
	h.Log.Info("csv export", "id", httpx.RequestID(ctx), "user_id", reqctx.User(ctx).ID, "rows", n)
}

// List is the "Exports" page: the admin's recent exports with progress.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := gen.New(h.Pool).ListUserExports(ctx, reqctx.User(ctx).ID)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	vm := pages.ExportsVM{}
	for _, e := range rows {
		x := pages.ExportRow{ID: e.ID, Status: e.Status, Total: int(e.TotalRows), Done: int(e.DoneRows),
			Created: e.CreatedAt, Query: e.Query}
		if e.FileName != nil {
			x.FileName = *e.FileName
		}
		if e.FileSize != nil {
			x.Size = *e.FileSize
		}
		if e.Error != nil {
			x.Error = *e.Error
		}
		if e.Status == "queued" || e.Status == "running" {
			vm.AnyPending = true
		}
		vm.Rows = append(vm.Rows, x)
	}
	web.OK(w, r, pages.Exports(vm))
}

// Download serves a finished export (only to the admin who made it).
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		web.NotFound(w, r)
		return
	}
	f, err := gen.New(h.Pool).GetExportFile(ctx, gen.GetExportFileParams{ID: id, UserID: reqctx.User(ctx).ID})
	if errors.Is(err, pgx.ErrNoRows) {
		web.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(f.File))
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	name := "applications.csv"
	if f.FileName != nil {
		name = *f.FileName
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, zr); err != nil {
		h.Log.Error("export download", "export_id", id, "err", err)
		panic(http.ErrAbortHandler)
	}
}

// statusStopPolling tells htmx to stop polling (https://htmx.org/attributes/hx-trigger/).
const statusStopPolling = 286

// Notify is polled by admin pages. It returns a notice for each export that
// finished since the last poll, and tells htmx to stop polling once nothing
// is pending, so idle pages make a single cheap request.
func (h *Handler) Notify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := reqctx.User(ctx).ID
	q := gen.New(h.Pool)
	ready, err := q.TakeExportNotifications(ctx, uid)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	pending, err := q.CountPendingExports(ctx, uid)
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	if len(ready) == 0 && pending == 0 {
		w.WriteHeader(statusStopPolling)
		return
	}
	var notes []pages.ExportNotice
	for _, e := range ready {
		notes = append(notes, pages.ExportNotice{ID: e.ID, OK: e.Status == "done", Rows: int(e.TotalRows)})
	}
	web.OK(w, r, pages.ExportNotices(notes))
}
