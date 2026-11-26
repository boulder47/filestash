package plg_handler_locks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/mickael-kerjean/filestash/server/common"
)

func TestResourceKeyUserIndependent(t *testing.T) {
	alice := map[string]string{"type": "samba", "host": "nas", "share": "docs", "user": "alice"}
	bob := map[string]string{"type": "samba", "host": "nas", "share": "docs", "user": "bob"}
	if resourceKey(alice, "/docs/f.txt") != resourceKey(bob, "/docs/f.txt") {
		t.Error("same server+path should yield same resource for different users")
	}
	other := map[string]string{"type": "samba", "host": "other", "share": "docs", "user": "alice"}
	if resourceKey(alice, "/docs/f.txt") == resourceKey(other, "/docs/f.txt") {
		t.Error("different host should yield a different resource")
	}
}

func TestTtlFrom(t *testing.T) {
	mk := func(q string) *http.Request { return httptest.NewRequest("POST", "/api/lock?"+q, nil) }
	if ttlFrom(mk("")) != defaultLockTTL {
		t.Error("missing ttl should default")
	}
	if ttlFrom(mk("ttl=30")) != 30*1e9 {
		t.Error("ttl=30 should be 30s")
	}
	if ttlFrom(mk("ttl=99999")) != maxLockTTL {
		t.Error("oversized ttl should clamp to max")
	}
}

func TestHandleLockAcquireThenConflict(t *testing.T) {
	manager = newLockManager() // reset package global
	alice := &App{Session: map[string]string{"type": "samba", "host": "nas", "share": "s", "user": "alice"}}
	bob := &App{Session: map[string]string{"type": "samba", "host": "nas", "share": "s", "user": "bob"}}

	rec := httptest.NewRecorder()
	handleLock(alice, rec, httptest.NewRequest("POST", "/api/lock?path=/s/f.txt", nil))
	if body := rec.Body.String(); !strings.Contains(body, `"granted": true`) || !strings.Contains(body, `"token": "`) {
		t.Fatalf("alice acquire response = %s", body)
	}

	rec = httptest.NewRecorder()
	handleLock(bob, rec, httptest.NewRequest("POST", "/api/lock?path=/s/f.txt", nil))
	if body := rec.Body.String(); !strings.Contains(body, `"granted": false`) {
		t.Errorf("bob should be denied, got %s", body)
	}

	// alice sees it as hers
	rec = httptest.NewRecorder()
	handleLock(alice, rec, httptest.NewRequest("GET", "/api/lock?path=/s/f.txt", nil))
	if body := rec.Body.String(); !strings.Contains(body, `"locked": true`) || !strings.Contains(body, `"mine": true`) {
		t.Errorf("alice GET should show mine=true, got %s", body)
	}
}

func TestHandleLockMissingPath(t *testing.T) {
	app := &App{Session: map[string]string{"type": "samba"}}
	rec := httptest.NewRecorder()
	handleLock(app, rec, httptest.NewRequest("POST", "/api/lock", nil))
	if rec.Code == http.StatusOK {
		t.Error("missing path should not return 200 OK")
	}
}
