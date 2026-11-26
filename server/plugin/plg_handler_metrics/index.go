// Package plg_handler_metrics exposes a Prometheus-compatible /metrics endpoint.
//
// It is intentionally dependency-free (standard library only) and emits the
// Prometheus text exposition format by hand, so it adds no modules to go.mod.
// Swapping in prometheus/client_golang later is a drop-in enhancement.
//
// Scope note: /metrics is served unauthenticated (the usual Prometheus model);
// on an internal deployment keep it reachable only from your scrape host.
package plg_handler_metrics

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/mickael-kerjean/filestash/server/common"
	"github.com/mickael-kerjean/filestash/server/pkg/env"

	"github.com/gorilla/mux"
)

var (
	startTime = time.Now()
	inFlight  int64

	mu          sync.Mutex
	reqByMethod = map[string]int64{}
	durSum      float64 // sum of request durations, seconds
	durCount    int64
)

func init() {
	Hooks.Register.Middleware(metricsMiddleware)
	Hooks.Register.HttpEndpoint(func(r *mux.Router) error {
		r.HandleFunc(WithBase("/metrics"), func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			writeMetrics(w)
		}).Methods("GET")
		return nil
	})
}

// metricsMiddleware records per-method counts and request durations. It does not
// wrap http.ResponseWriter, so Flusher/Hijacker behaviour (SSE, websockets) is
// untouched; status-code labels are intentionally left for a later iteration.
func metricsMiddleware(next HandlerFunc) HandlerFunc {
	return func(app *App, res http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&inFlight, 1)
		start := time.Now()
		next(app, res, req)
		elapsed := time.Since(start).Seconds()
		atomic.AddInt64(&inFlight, -1)

		mu.Lock()
		reqByMethod[req.Method]++
		durSum += elapsed
		durCount++
		mu.Unlock()
	}
}

func writeMetrics(w io.Writer) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	mu.Lock()
	methods := make([]string, 0, len(reqByMethod))
	for m := range reqByMethod {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	counts := make([]int64, len(methods))
	for i, m := range methods {
		counts[i] = reqByMethod[m]
	}
	sum, count := durSum, durCount
	mu.Unlock()

	fmt.Fprintf(w, "# HELP filestash_build_info Build information.\n")
	fmt.Fprintf(w, "# TYPE filestash_build_info gauge\n")
	fmt.Fprintf(w, "filestash_build_info{version=%q} 1\n", env.APP_VERSION)

	fmt.Fprintf(w, "# HELP filestash_uptime_seconds Seconds since process start.\n")
	fmt.Fprintf(w, "# TYPE filestash_uptime_seconds gauge\n")
	fmt.Fprintf(w, "filestash_uptime_seconds %g\n", time.Since(startTime).Seconds())

	fmt.Fprintf(w, "# HELP filestash_http_requests_total Total HTTP requests handled, by method.\n")
	fmt.Fprintf(w, "# TYPE filestash_http_requests_total counter\n")
	for i, m := range methods {
		fmt.Fprintf(w, "filestash_http_requests_total{method=%q} %d\n", m, counts[i])
	}

	fmt.Fprintf(w, "# HELP filestash_http_requests_in_flight In-flight HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE filestash_http_requests_in_flight gauge\n")
	fmt.Fprintf(w, "filestash_http_requests_in_flight %d\n", atomic.LoadInt64(&inFlight))

	fmt.Fprintf(w, "# HELP filestash_http_request_duration_seconds_sum Cumulative request duration.\n")
	fmt.Fprintf(w, "# TYPE filestash_http_request_duration_seconds_sum counter\n")
	fmt.Fprintf(w, "filestash_http_request_duration_seconds_sum %g\n", sum)
	fmt.Fprintf(w, "# HELP filestash_http_request_duration_seconds_count Count of timed requests.\n")
	fmt.Fprintf(w, "# TYPE filestash_http_request_duration_seconds_count counter\n")
	fmt.Fprintf(w, "filestash_http_request_duration_seconds_count %d\n", count)

	fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines currently running.\n")
	fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
	fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP go_memstats_alloc_bytes Allocated and in-use heap bytes.\n")
	fmt.Fprintf(w, "# TYPE go_memstats_alloc_bytes gauge\n")
	fmt.Fprintf(w, "go_memstats_alloc_bytes %d\n", ms.Alloc)
}
