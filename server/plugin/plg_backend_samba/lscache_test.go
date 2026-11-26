package plg_backend_samba

import (
	"os"
	"testing"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
)

func TestParentDir(t *testing.T) {
	cases := map[string]string{
		"/share/dir/file.txt": "/share/dir",
		"/share/newdir":       "/share",
		"/share":              "/",
		"/share/dir/":         "/share",
		"/":                   "/",
	}
	for in, want := range cases {
		if got := parentDir(in); got != want {
			t.Errorf("parentDir(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLsCacheGetSetExpire(t *testing.T) {
	c := newLsCache()
	data := []os.FileInfo{File{FName: "a", FType: "file"}}
	if _, ok := c.get("/x"); ok {
		t.Fatal("empty cache should miss")
	}
	c.set("/x", data, time.Minute)
	got, ok := c.get("/x")
	if !ok || len(got) != 1 || got[0].Name() != "a" {
		t.Fatalf("hit failed: ok=%v got=%v", ok, got)
	}
	c.set("/y", data, -time.Second) // already expired
	if _, ok := c.get("/y"); ok {
		t.Error("expired entry should miss")
	}
}

func TestLsCacheInvalidate(t *testing.T) {
	c := newLsCache()
	c.set("/share", []os.FileInfo{File{FName: "a"}}, time.Minute)
	c.set("/share/dir", []os.FileInfo{File{FName: "b"}}, time.Minute)
	c.invalidate("/share")
	if _, ok := c.get("/share"); ok {
		t.Error("/share should be invalidated")
	}
	if _, ok := c.get("/share/dir"); !ok {
		t.Error("/share/dir should remain cached")
	}
}
