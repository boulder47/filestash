package coretest

import (
	"io"
	"strings"
	"testing"
)

func TestFakeBackendSaveCatStat(t *testing.T) {
	b := NewFakeBackend()
	if err := b.Save("/dir/hello.txt", strings.NewReader("hi")); err != nil {
		t.Fatal(err)
	}
	fi, err := b.Stat("/dir/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Name() != "hello.txt" || fi.Size() != 2 {
		t.Errorf("Stat = (%q,%d), want (hello.txt,2)", fi.Name(), fi.Size())
	}
	rc, err := b.Cat("/dir/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hi" {
		t.Errorf("Cat = %q, want hi", data)
	}
}

func TestFakeBackendLsAndMvAndRm(t *testing.T) {
	b := NewFakeBackend()
	_ = b.Save("/a.txt", strings.NewReader("1"))
	_ = b.Save("/sub/b.txt", strings.NewReader("22"))

	entries, err := b.Ls("/")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, e := range entries {
		typ := "file"
		if e.IsDir() {
			typ = "dir"
		}
		names[e.Name()] = typ
	}
	if names["a.txt"] != "file" || names["sub"] != "dir" {
		t.Errorf("Ls(/) = %v, want a.txt=file sub=dir", names)
	}

	if err := b.Mv("/a.txt", "/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Stat("/a.txt"); err == nil {
		t.Error("source still present after Mv")
	}
	if _, err := b.Stat("/renamed.txt"); err != nil {
		t.Errorf("destination missing after Mv: %v", err)
	}

	if err := b.Rm("/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Stat("/renamed.txt"); err == nil {
		t.Error("file still present after Rm")
	}
}
