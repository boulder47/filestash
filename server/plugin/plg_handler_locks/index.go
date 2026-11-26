// Package plg_handler_locks provides an advisory file-locking service (SMB-3)
// so clients can coordinate edits and avoid clobbering a shared file.
//
// It is ADVISORY only: it adds endpoints under /api/lock and /api/locks and does
// NOT enforce anything at the storage layer, so it cannot block legitimate I/O.
// A cooperating client (e.g. the editor) acquires a lock before editing, refreshes
// it periodically, releases it when done, and warns the user on a conflict.
//
// Locks are keyed by the logical resource (server+share+path, user-independent),
// so two different users editing the same file collide; the owner is the
// non-reversible GenerateID hash of the session.
package plg_handler_locks

import (
	"net/http"
	"strconv"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
	. "github.com/mickael-kerjean/filestash/server/middleware"

	"github.com/gorilla/mux"
)

const (
	defaultLockTTL = 60 * time.Second
	maxLockTTL     = 3600 * time.Second
)

var manager = newLockManager()

func init() {
	Hooks.Register.HttpEndpoint(func(r *mux.Router) error {
		m := []Middleware{ApiHeaders, SecureHeaders, SessionStart, LoggedInOnly}
		r.HandleFunc(WithBase("/api/lock"), NewMiddlewareChain(handleLock, m)).Methods("GET", "POST", "PUT", "DELETE")
		r.HandleFunc(WithBase("/api/locks"), NewMiddlewareChain(handleList, m)).Methods("GET")
		return nil
	})
}

// resourceKey is a user-independent id for a logical file: server + share come
// from the session, the file path from the request. Credentials are excluded so
// different users of the same share collide on the same resource.
func resourceKey(session map[string]string, path string) string {
	return QuickHash(session["type"]+"|"+session["host"]+"|"+session["share"], 16) + ":" + path
}

func ttlFrom(req *http.Request) time.Duration {
	if s := req.URL.Query().Get("ttl"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			d := time.Duration(n) * time.Second
			if d > maxLockTTL {
				return maxLockTTL
			}
			return d
		}
	}
	return defaultLockTTL
}

func handleLock(app *App, res http.ResponseWriter, req *http.Request) {
	path := req.URL.Query().Get("path")
	if path == "" {
		SendErrorResult(res, ErrNotValid)
		return
	}
	resource := resourceKey(app.Session, path)
	owner := GenerateID(app.Session)

	switch req.Method {
	case http.MethodGet:
		out := map[string]interface{}{"locked": false}
		if l, ok := manager.get(resource); ok {
			out["locked"] = true
			out["mine"] = l.Owner == owner
			out["owner"] = l.OwnerName
			out["expiry"] = l.Expiry.Unix()
		}
		SendSuccessResult(res, out)
	case http.MethodPost:
		l, conflict := manager.acquire(resource, owner, app.Session["user"], ttlFrom(req))
		if conflict != nil {
			SendSuccessResult(res, map[string]interface{}{
				"granted": false, "owner": conflict.OwnerName, "expiry": conflict.Expiry.Unix(),
			})
			return
		}
		SendSuccessResult(res, map[string]interface{}{
			"granted": true, "token": l.Token, "expiry": l.Expiry.Unix(),
		})
	case http.MethodPut:
		l, ok := manager.refresh(resource, req.URL.Query().Get("token"), ttlFrom(req))
		if !ok {
			SendErrorResult(res, ErrNotFound)
			return
		}
		SendSuccessResult(res, map[string]interface{}{"granted": true, "expiry": l.Expiry.Unix()})
	case http.MethodDelete:
		if !manager.release(resource, req.URL.Query().Get("token")) {
			SendErrorResult(res, ErrPermissionDenied)
			return
		}
		SendSuccessResult(res, map[string]interface{}{"released": true})
	}
}

func handleList(app *App, res http.ResponseWriter, req *http.Request) {
	owner := GenerateID(app.Session)
	out := []map[string]interface{}{}
	for _, l := range manager.list(owner) {
		out = append(out, map[string]interface{}{"owner": l.OwnerName, "expiry": l.Expiry.Unix()})
	}
	SendSuccessResult(res, out)
}
