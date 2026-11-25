package utils

import "testing"

func TestJoinPathBlocksTraversal(t *testing.T) {
	cases := []struct{ base, file, want string }{
		{"/a", "b", "/a/b"},
		{"/a", "b/c", "/a/b/c"},
		{"/a", "../etc/passwd", "/a"}, // escape attempt -> clamped to base
		{"/a", "..", "/a"},
		{"/a/b", "../b/c", "/a/b/c"},
	}
	for _, c := range cases {
		if got := JoinPath(c.base, c.file); got != c.want {
			t.Errorf("JoinPath(%q,%q) = %q, want %q", c.base, c.file, got, c.want)
		}
	}
}

func TestEnforceDirectory(t *testing.T) {
	cases := map[string]string{"": "/", "/a": "/a/", "/a/": "/a/", "/a/b": "/a/b/"}
	for in, want := range cases {
		if got := EnforceDirectory(in); got != want {
			t.Errorf("EnforceDirectory(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitPath(t *testing.T) {
	cases := []struct{ in, root, file string }{
		{"", "/", ""},
		{"/", "/", ""},
		{"/a/b/", "/a/b/", ""},
		{"/a/b.txt", "/a/", "b.txt"},
		{"/file.txt", "/", "file.txt"},
	}
	for _, c := range cases {
		root, file := SplitPath(c.in)
		if root != c.root || file != c.file {
			t.Errorf("SplitPath(%q) = (%q,%q), want (%q,%q)", c.in, root, file, c.root, c.file)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.txt", "a.txt", true},
		{"*.txt", "a.md", false},
		{"**/*.go", "server/pkg/x.go", true},
		{"foo/*", "foo/bar", true},
		{"foo/*", "foo/bar/baz", false},
	}
	for _, c := range cases {
		if got := GlobMatch(c.pattern, c.name); got != c.want {
			t.Errorf("GlobMatch(%q,%q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestIsDirectory(t *testing.T) {
	if IsDirectory("") || IsDirectory("/a") {
		t.Error("IsDirectory should be false for empty and non-slash-terminated paths")
	}
	if !IsDirectory("/") || !IsDirectory("/a/") {
		t.Error("IsDirectory should be true for slash-terminated paths")
	}
}
