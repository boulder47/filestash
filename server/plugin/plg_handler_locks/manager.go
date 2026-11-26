package plg_handler_locks

import (
	"sync"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
)

const maxLocks = 4096

// Lock is an advisory lock on a logical resource (server+share+path), held by a
// session owner. Owner is the non-reversible GenerateID hash; OwnerName is the
// display username. Token is the secret needed to refresh/release.
type Lock struct {
	Resource  string
	Owner     string
	OwnerName string
	Token     string
	Expiry    time.Time
}

type lockManager struct {
	mu    sync.Mutex
	locks map[string]Lock
}

func newLockManager() *lockManager { return &lockManager{locks: map[string]Lock{}} }

// acquire grants or refreshes a lock. On success returns the lock and nil. If
// another, unexpired owner holds it, returns a copy of that conflicting lock.
func (m *lockManager) acquire(resource, owner, ownerName string, ttl time.Duration) (Lock, *Lock) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if len(m.locks) >= maxLocks {
		m.purgeExpiredLocked(now)
	}
	existing, ok := m.locks[resource]
	if ok && now.Before(existing.Expiry) && existing.Owner != owner {
		c := existing
		return Lock{}, &c
	}
	token := existing.Token
	if !ok || now.After(existing.Expiry) || existing.Owner != owner || token == "" {
		token = RandomString(24)
	}
	l := Lock{Resource: resource, Owner: owner, OwnerName: ownerName, Token: token, Expiry: now.Add(ttl)}
	m.locks[resource] = l
	return l, nil
}

func (m *lockManager) refresh(resource, token string, ttl time.Duration) (Lock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	l, ok := m.locks[resource]
	if !ok || now.After(l.Expiry) || l.Token != token {
		return Lock{}, false
	}
	l.Expiry = now.Add(ttl)
	m.locks[resource] = l
	return l, true
}

func (m *lockManager) release(resource, token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[resource]
	if !ok {
		return true // already released
	}
	if l.Token != token {
		return false
	}
	delete(m.locks, resource)
	return true
}

func (m *lockManager) get(resource string) (Lock, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[resource]
	if !ok || time.Now().After(l.Expiry) {
		return Lock{}, false
	}
	return l, true
}

func (m *lockManager) list(owner string) []Lock {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	out := []Lock{}
	for _, l := range m.locks {
		if now.Before(l.Expiry) && l.Owner == owner {
			out = append(out, l)
		}
	}
	return out
}

func (m *lockManager) purgeExpiredLocked(now time.Time) {
	for k, l := range m.locks {
		if now.After(l.Expiry) {
			delete(m.locks, k)
		}
	}
}
