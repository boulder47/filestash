package plg_audit_log

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
)

type auditViewer struct{ store *Store }

func (v auditViewer) Query(ctx *App, p map[string]string) (AuditQueryResult, error) {
	f := Filter{
		Action:  p["action"],
		Path:    p["path"],
		Backend: p["backend"],
		Session: p["session"],
		User:    p["user"],
		Target:  p["target"],
		From:    parseTime(p["date_from"]),
		To:      parseTime(p["date_to"]),
		Limit:   500,
	}
	form := filterForm()
	if v.store == nil {
		return AuditQueryResult{Form: &form, RenderHTML: "<div>audit store unavailable</div>"}, nil
	}
	events, err := v.store.Query(f)
	if err != nil {
		return AuditQueryResult{}, err
	}
	return AuditQueryResult{Form: &form, RenderHTML: renderTable(events)}, nil
}

func filterForm() Form {
	return Form{
		Form: []Form{{
			Title: "search",
			Elmnts: []FormElement{
				{Name: "date_from", Type: "datetime"},
				{Name: "date_to", Type: "datetime"},
				{Name: "action", Type: "select", Opts: []string{"", "create_file", "create_folder", "save_file", "move", "rename", "remove"}},
				{Name: "path", Type: "text"},
				{Name: "target", Type: "text"},
				{Name: "backend", Type: "text"},
				{Name: "user", Type: "text"},
				{Name: "session", Type: "text"},
			},
		}},
	}
}

func renderTable(events []Event) string {
	var b strings.Builder
	b.WriteString(`<style>
        table.audit{width:100%;border-collapse:collapse;font-size:0.85rem}
        table.audit th,table.audit td{text-align:left;padding:6px 10px;border-bottom:1px solid var(--border,#444)}
        table.audit td.path{font-family:monospace;word-break:break-all}
    </style>`)
	b.WriteString(fmt.Sprintf(`<div class="audit-count">%d entr%s</div>`, len(events), plural(len(events))))
	if len(events) == 0 {
		return b.String() + `<div>No audit entries match.</div>`
	}
	b.WriteString(`<table class="audit"><thead><tr>` +
		`<th>time</th><th>action</th><th>user</th><th>backend</th><th>path</th><th>target</th><th>size</th>` +
		`</tr></thead><tbody>`)
	for _, e := range events {
		target := e.Target
		if target == "" {
			target = "-"
		}
		b.WriteString("<tr>")
		b.WriteString("<td>" + html.EscapeString(time.Unix(e.Timestamp, 0).UTC().Format("2006-01-02 15:04:05")) + "</td>")
		b.WriteString("<td>" + html.EscapeString(e.Action) + "</td>")
		b.WriteString("<td>" + html.EscapeString(orDash(e.User)) + "</td>")
		b.WriteString("<td>" + html.EscapeString(orDash(e.Backend)) + "</td>")
		b.WriteString(`<td class="path">` + html.EscapeString(e.Path) + "</td>")
		b.WriteString(`<td class="path">` + html.EscapeString(target) + "</td>")
		b.WriteString("<td>" + html.EscapeString(sizeStr(e.Size)) + "</td>")
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func sizeStr(n int64) string {
	if n <= 0 {
		return "-"
	}
	return strconv.FormatInt(n, 10)
}

// parseTime accepts unix seconds, RFC3339, or an HTML datetime-local value.
func parseTime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}
