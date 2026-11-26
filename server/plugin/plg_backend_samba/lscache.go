package plg_backend_samba

import (
	"os"
	"strings"
	"sync"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
)

const (
	defaultLsCacheSeconds = 10
	maxLsCacheEntries     = 2048
)

// lsCache is a tiny per-session, TTL'd cache of directory listings. It lives on
// a pooled *Samba, so it is naturally scoped to one connection/identity and dies
// when that session is evicted. Writes through the session invalidate the
// affected directory, so the only staleness window is cross-session writes
// within the (short) TTL.
type lsCache struct {
	mu      sync.Mutex
	entries map[string]lsEntry
}

type lsEntry struct {
	data []os.FileInfo
	exp  time.Time
}

func newLsCache() *lsCache { return &lsCache{entries: map[string]lsEntry{}} }

func (c *lsCache) get(key string) ([]os.FileInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return e.data, true
}

func (c *lsCache) set(key string, data []os.FileInfo, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxLsCacheEntries {
		c.purgeExpiredLocked()
	}
	c.entries[key] = lsEntry{data: data, exp: time.Now().Add(ttl)}
}

func (c *lsCache) invalidate(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.entries, k)
	}
}

func (c *lsCache) purgeExpiredLocked() {
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.exp) {
			delete(c.entries, k)
		}
	}
}

// parentDir returns the Ls cache key (the containing directory) of p, matching
// the path form callers pass to Ls (e.g. "/share/dir/file" -> "/share/dir").
func parentDir(p string) string {
	p = strings.TrimRight(p, "/")
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

// lsCacheTTL reads the configured cache window. Unset -> default; <=0 disables.
func lsCacheTTL() time.Duration {
	secs := defaultLsCacheSeconds
	switch v := Config.Get("samba.ls_cache_seconds").Interface().(type) {
	case float64:
		secs = int(v)
	case int:
		secs = v
	}
	if secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
