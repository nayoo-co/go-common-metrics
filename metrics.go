// Package metrics — NaYoo shared Prometheus client wrapper.
//
// Standard usage in a Gin service main.go:
//
//	import metrics "github.com/serpentdark/go-common-metrics"
//
//	func main() {
//	    metrics.Init("go-system-listing-service")
//	    go metrics.Serve(":9090")          // /metrics on a sidecar port
//	    r := gin.Default()
//	    r.Use(metrics.GinMiddleware())     // observe every HTTP request
//	    // ... routes ...
//	    r.Run(":" + os.Getenv("PORT"))
//	}
//
// Worker (no Gin):
//
//	metrics.Init("go-worker-listing-service")
//	go metrics.Serve(":9090")
//	// ... worker loop ...
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Registry is the package-level registry. Init() populates it with
	// default collectors (Go runtime + process) and the NaYoo HTTP counters.
	// Exposed so services can register their own custom business metrics:
	//   metrics.Registry.MustRegister(myBusinessCounter)
	Registry = prometheus.NewRegistry()

	// HTTPRequests counts every HTTP request handled, labeled by method,
	// route template (NOT raw URL — avoids cardinality blow-up), and status code.
	HTTPRequests *prometheus.CounterVec

	// HTTPDuration is a histogram of request handling latency in seconds.
	// Buckets cover < 1ms up to > 10s to fit typical web latencies.
	HTTPDuration *prometheus.HistogramVec

	// HTTPInflight tracks concurrent in-progress requests.
	HTTPInflight *prometheus.GaugeVec

	serviceName string
	initOnce    sync.Once
)

// Init must be called once at service start. It seeds the registry with:
//   - Go runtime collector (goroutines, GC, memstats)
//   - Process collector (RSS, CPU seconds, open FDs)
//   - NaYoo HTTP counters/histograms (no samples until middleware fires)
//
// The serviceName argument becomes the `service` label on every NaYoo
// metric — keep it stable across deployments of the same service.
func Init(name string) {
	initOnce.Do(func() {
		serviceName = name

		Registry.MustRegister(collectors.NewGoCollector())
		Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

		HTTPRequests = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "nayoo_http_requests_total",
				Help: "Total HTTP requests handled, labeled by method, route template, and status code.",
			},
			[]string{"service", "method", "path", "code"},
		)
		HTTPDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "nayoo_http_request_duration_seconds",
				Help: "HTTP request handling latency.",
				Buckets: []float64{
					0.001, 0.005, 0.01, 0.025, 0.05, 0.1,
					0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
				},
			},
			[]string{"service", "method", "path"},
		)
		HTTPInflight = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "nayoo_http_inflight_requests",
				Help: "In-progress HTTP requests being handled right now.",
			},
			[]string{"service"},
		)

		Registry.MustRegister(HTTPRequests, HTTPDuration, HTTPInflight)
	})
}

// ServiceName returns the name passed to Init() — useful for services that
// want to tag their own custom metrics with the same service label.
func ServiceName() string { return serviceName }

// Serve starts a dedicated HTTP server that exposes Prometheus metrics on
// `/metrics` at the given address. It's a blocking call — run in a goroutine.
// Intended for a sidecar-style port (e.g., :9090) separate from the main
// application port so the metrics endpoint isn't subject to auth/middleware
// configured on the Gin engine.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		Registry:          Registry,
		EnableOpenMetrics: false,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}
