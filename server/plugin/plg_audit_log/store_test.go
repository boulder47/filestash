package plg_audit_log

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "audit.sql"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestRecordAndQueryAll(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()
	for i, a := range []string{"create_folder", "remove", "create_file"} {
		if err := s.Record(Event{Timestamp: now + int64(i), Action: a, Session: "sess", Path: "/p"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Query(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0].Action != "create_file" { // newest first
		t.Errorf("ordering: newest = %q, want create_file", got[0].Action)
	}
}

func TestDedupSaveFile(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()
	// three rapid saves of the same file by the same session -> 1 row
	for i := 0; i < 3; i++ {
		if err := s.Record(Event{Timestamp: now + int64(i), Action: "save_file", Session: "sess", Path: "/a.txt"}); err != nil {
			t.Fatal(err)
		}
	}
	// a different path is a separate entry
	if err := s.Record(Event{Timestamp: now, Action: "save_file", Session: "sess", Path: "/b.txt"}); err != nil {
		t.Fatal(err)
	}
	// non-save action is never deduped
	_ = s.Record(Event{Timestamp: now, Action: "remove", Session: "sess", Path: "/a.txt"})
	_ = s.Record(Event{Timestamp: now + 1, Action: "remove", Session: "sess", Path: "/a.txt"})

	saves, _ := s.Query(Filter{Action: "save_file"})
	if len(saves) != 2 {
		t.Errorf("save_file rows = %d, want 2 (deduped /a.txt + /b.txt)", len(saves))
	}
	removes, _ := s.Query(Filter{Action: "remove"})
	if len(removes) != 2 {
		t.Errorf("remove rows = %d, want 2 (not deduped)", len(removes))
	}
}

func TestPruneByAgeAndCount(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()
	_ = s.Record(Event{Timestamp: now - 100*86400, Action: "remove", Path: "/old"})
	_ = s.Record(Event{Timestamp: now, Action: "remove", Path: "/new"})
	if err := s.Prune(90, 0); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Query(Filter{})
	if len(got) != 1 || got[0].Path != "/new" {
		t.Fatalf("after age prune: %+v, want only /new", got)
	}

	for i := 0; i < 5; i++ {
		_ = s.Record(Event{Timestamp: now + int64(i), Action: "remove", Path: "/x"})
	}
	if err := s.Prune(0, 3); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Query(Filter{})
	if len(got) != 3 {
		t.Errorf("after count prune: %d rows, want 3", len(got))
	}
}

func TestQueryFilters(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()
	_ = s.Record(Event{Timestamp: now - 10, Action: "remove", Path: "/docs/a.txt", Backend: "samba"})
	_ = s.Record(Event{Timestamp: now, Action: "create_folder", Path: "/photos", Backend: "s3"})

	if got, _ := s.Query(Filter{Action: "remove"}); len(got) != 1 || got[0].Path != "/docs/a.txt" {
		t.Errorf("action filter: %+v", got)
	}
	if got, _ := s.Query(Filter{Path: "docs"}); len(got) != 1 {
		t.Errorf("path LIKE filter returned %d, want 1", len(got))
	}
	if got, _ := s.Query(Filter{From: now - 1}); len(got) != 1 || got[0].Path != "/photos" {
		t.Errorf("time From filter: %+v", got)
	}
}
