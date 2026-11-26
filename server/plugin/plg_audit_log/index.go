// Package plg_audit_log records file-change events (create/rename/delete/modify)
// to a local sqlite store and exposes them through the admin panel's audit view
// (IAuditPlugin). It implements SEC-1 of docs/IMPLEMENTATION_PLAN.md.
//
// Reads (list/download) are intentionally NOT audited by default — only
// mutations. User/session identifiers are the non-reversible GenerateID hash;
// no password or raw credential is stored.
package plg_audit_log

import (
	"database/sql"
	"net/http"
	"path"
	"strings"
	"sync/atomic"

	. "github.com/mickael-kerjean/filestash/server/common"
	_ "github.com/mickael-kerjean/filestash/server/pkg/sqlite"
)

const (
	defaultRetentionDays = 90
	defaultMaxEntries    = 100000
	pruneEvery           = 256
)

var (
	store       *Store
	insertCount atomic.Int64
)

func init() {
	db, err := sql.Open("sqlite3", GetAbsolutePath(DB_PATH, "audit.sql"))
	if err != nil {
		Log.Error("plg_audit_log - cannot open audit db: %s", err.Error())
		return
	}
	store, err = NewStore(db)
	if err != nil {
		Log.Error("plg_audit_log - cannot init audit schema: %s", err.Error())
		store = nil
		return
	}
	Hooks.Register.Middleware(AuditLog)
	Hooks.Register.Onload(func() {
		declareConfig()
		// Registered in Onload so it overrides the default SimpleAudit viewer
		// regardless of plugin init order.
		Hooks.Register.AuditEngine(auditViewer{store: store})
		store.Prune(retentionDays(), maxEntries())
	})
}

// AuditLog captures successful file-mutating requests. It reads the chain's
// status via the Status() method the core ResponseWriter exposes, so it does
// not wrap the writer (SSE/websocket handlers are untouched).
func AuditLog(next HandlerFunc) HandlerFunc {
	return func(app *App, res http.ResponseWriter, req *http.Request) {
		action, ok := actionFor(req.Method, req.URL.Path)
		next(app, res, req)
		if !ok || store == nil {
			return
		}
		if sw, k := res.(interface{ Status() int }); k && sw.Status() >= 400 {
			return
		}
		q := req.URL.Query()
		ev := Event{
			User:      app.Session["user"],
			Action:    action,
			Backend:   app.Session["type"],
			Session:   GenerateID(app.Session),
			RequestID: res.Header().Get("X-Request-Id"),
		}
		if action == "move" {
			ev.Path = q.Get("from")
			ev.Target = q.Get("to")
			if path.Dir(ev.Path) == path.Dir(ev.Target) {
				ev.Action = "rename"
			}
		} else {
			ev.Path = q.Get("path")
		}
		if err := store.Record(ev); err != nil {
			Log.Warning("plg_audit_log - record: %s", err.Error())
			return
		}
		if insertCount.Add(1)%pruneEvery == 0 {
			store.Prune(retentionDays(), maxEntries())
		}
	}
}

// actionFor maps a file-API request to a canonical audit action (matching the
// action vocabulary the admin audit form already uses). Only mutations map.
func actionFor(method, urlPath string) (string, bool) {
	if !strings.Contains(urlPath, "/api/files/") {
		return "", false
	}
	seg := urlPath[strings.LastIndex(urlPath, "/")+1:]
	switch {
	case method == http.MethodPost && seg == "mkdir":
		return "create_folder", true
	case method == http.MethodPost && seg == "touch":
		return "create_file", true
	case method == http.MethodPost && seg == "rm":
		return "remove", true
	case method == http.MethodPost && seg == "mv":
		return "move", true
	case (method == http.MethodPost || method == http.MethodPatch) && (seg == "save" || seg == "cat"):
		return "save_file", true
	}
	return "", false
}

func declareConfig() {
	Config.Get("audit.retention_days").Schema(func(f *FormElement) *FormElement {
		if f == nil {
			f = &FormElement{}
		}
		f.Id = "audit_retention_days"
		f.Name = "retention_days"
		f.Type = "number"
		f.Default = defaultRetentionDays
		f.Description = "How many days to keep audit-log entries"
		return f
	})
	Config.Get("audit.max_entries").Schema(func(f *FormElement) *FormElement {
		if f == nil {
			f = &FormElement{}
		}
		f.Id = "audit_max_entries"
		f.Name = "max_entries"
		f.Type = "number"
		f.Default = defaultMaxEntries
		f.Description = "Maximum number of audit-log entries to retain"
		return f
	})
}

func retentionDays() int {
	if v := Config.Get("audit.retention_days").Int(); v > 0 {
		return v
	}
	return defaultRetentionDays
}

func maxEntries() int {
	if v := Config.Get("audit.max_entries").Int(); v > 0 {
		return v
	}
	return defaultMaxEntries
}
