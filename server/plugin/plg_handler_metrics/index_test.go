package plg_handler_metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/mickael-kerjean/filestash/server/common"
)

func resetState() {
	mu.Lock()
	reqByMethod = map[string]int64{}
	durSum = 0
	durCount = 0
	mu.Unlock()
}

func TestWriteMetricsExposesExpectedFamilies(t *testing.T) {
	resetState()
	var sb strings.Builder
	writeMetrics(&sb)
	out := sb.String()
	for _, want := range []string{
		"# TYPE filestash_build_info gauge",
		"filestash_build_info{version=",
		"# TYPE filestash_http_requests_total counter",
		"filestash_http_requests_in_flight",
		"filestash_http_request_duration_seconds_count",
		"go_goroutines",
		"go_memstats_alloc_bytes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q\n---\n%s", want, out)
		}
	}
}

func TestMiddlewareCountsRequestsByMethod(t *testing.T) {
	resetState()
	// a no-op downstream handler
	final := func(app *App, res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
	}
	wrapped := metricsMiddleware(final)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		wrapped(&App{}, httptest.NewRecorder(), req)
	}
	req := httptest.NewRequest(http.MethodPost, "/y", nil)
	wrapped(&App{}, httptest.NewRecorder(), req)

	mu.Lock()
	got := reqByMethod
	count := durCount
	mu.Unlock()

	if got["GET"] != 3 || got["POST"] != 1 {
		t.Errorf("counts = %v, want GET=3 POST=1", got)
	}
	if count != 4 {
		t.Errorf("durCount = %d, want 4", count)
	}

	var sb strings.Builder
	writeMetrics(&sb)
	if !strings.Contains(sb.String(), `filestash_http_requests_total{method="GET"} 3`) {
		t.Errorf("exposition missing GET=3 counter:\n%s", sb.String())
	}
}
