package plg_handler_locks

import (
	"testing"
	"time"
)

func TestAcquireGrantAndConflict(t *testing.T) {
	m := newLockManager()
	l, conflict := m.acquire("r1", "alice", "Alice", time.Minute)
	if conflict != nil || l.Token == "" {
		t.Fatalf("alice acquire failed: conflict=%v token=%q", conflict, l.Token)
	}
	if _, conflict := m.acquire("r1", "bob", "Bob", time.Minute); conflict == nil || conflict.OwnerName != "Alice" {
		t.Fatalf("expected conflict owned by Alice, got %v", conflict)
	}
	l2, conflict := m.acquire("r1", "alice", "Alice", time.Minute)
	if conflict != nil || l2.Token != l.Token {
		t.Fatalf("alice re-acquire should keep token: got %q want %q conflict=%v", l2.Token, l.Token, conflict)
	}
}

func TestAcquireAfterExpiry(t *testing.T) {
	m := newLockManager()
	m.acquire("r", "alice", "Alice", -time.Second) // already expired
	l, conflict := m.acquire("r", "bob", "Bob", time.Minute)
	if conflict != nil || l.Owner != "bob" {
		t.Fatalf("bob should acquire expired lock: conflict=%v owner=%q", conflict, l.Owner)
	}
}

func TestRefresh(t *testing.T) {
	m := newLockManager()
	l, _ := m.acquire("r", "alice", "Alice", time.Minute)
	if _, ok := m.refresh("r", "wrong-token", time.Minute); ok {
		t.Error("refresh with wrong token should fail")
	}
	nl, ok := m.refresh("r", l.Token, 2*time.Minute)
	if !ok || !nl.Expiry.After(l.Expiry) {
		t.Errorf("refresh should extend expiry: ok=%v", ok)
	}
}

func TestRelease(t *testing.T) {
	m := newLockManager()
	l, _ := m.acquire("r", "alice", "Alice", time.Minute)
	if m.release("r", "wrong-token") {
		t.Error("release with wrong token should fail")
	}
	if !m.release("r", l.Token) {
		t.Error("release with right token should succeed")
	}
	if _, ok := m.get("r"); ok {
		t.Error("lock should be gone after release")
	}
	if !m.release("r", "anything") {
		t.Error("releasing a missing lock should be a no-op success")
	}
}

func TestGetExpiredAndList(t *testing.T) {
	m := newLockManager()
	m.acquire("a", "alice", "Alice", time.Minute)
	m.acquire("b", "bob", "Bob", time.Minute)
	m.acquire("c", "alice", "Alice", -time.Second) // expired
	if _, ok := m.get("c"); ok {
		t.Error("expired lock should read as not locked")
	}
	locks := m.list("alice")
	if len(locks) != 1 || locks[0].Resource != "a" {
		t.Errorf("alice should hold exactly 'a', got %+v", locks)
	}
}
