package plg_audit_log

import (
	"net/http"
	"strings"
	"testing"
)

func TestActionFor(t *testing.T) {
	cases := []struct {
		method, path string
		want         string
		ok           bool
	}{
		{http.MethodPost, "/api/files/mkdir", "create_folder", true},
		{http.MethodPost, "/api/files/touch", "create_file", true},
		{http.MethodPost, "/api/files/rm", "remove", true},
		{http.MethodPost, "/api/files/mv", "move", true},
		{http.MethodPost, "/api/files/save", "save_file", true},
		{http.MethodPatch, "/api/files/cat", "save_file", true},
		// reads are not audited
		{http.MethodGet, "/api/files/ls", "", false},
		{http.MethodGet, "/api/files/cat", "", false},
		// unrelated endpoints
		{http.MethodPost, "/api/session", "", false},
		{http.MethodGet, "/metrics", "", false},
	}
	for _, c := range cases {
		got, ok := actionFor(c.method, c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("actionFor(%s,%s) = (%q,%v), want (%q,%v)", c.method, c.path, got, ok, c.want, c.ok)
		}
	}
}

func TestActionForRespectsBasePrefix(t *testing.T) {
	// WithBase may prepend a path prefix; matching must still work.
	if got, ok := actionFor(http.MethodPost, "/filestash/api/files/rm"); got != "remove" || !ok {
		t.Errorf("with base prefix = (%q,%v), want (remove,true)", got, ok)
	}
}

func TestParseTime(t *testing.T) {
	if parseTime("") != 0 {
		t.Error("empty should be 0")
	}
	if parseTime("1700000000") != 1700000000 {
		t.Error("unix seconds not parsed")
	}
	if parseTime("2023-01-02T03:04") == 0 {
		t.Error("datetime-local not parsed")
	}
	if parseTime("not-a-date") != 0 {
		t.Error("garbage should be 0")
	}
}

func TestRenderTableEscapesHTML(t *testing.T) {
	out := renderTable([]Event{{Timestamp: 1700000000, Action: "remove", Path: "/x/<script>alert(1)</script>"}})
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("path was not HTML-escaped in audit render")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected escaped path in output")
	}
}
