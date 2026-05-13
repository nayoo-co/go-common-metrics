// Package metrics — NaYoo standard Prometheus client wrapper.
//
// One library, three entry points — pick the one that matches your service type:
//
//	BFF        → metrics.InitBFF("go-process-nayoo-service")
//	System     → metrics.InitSystem("go-system-listing-service")
//	Worker     → metrics.InitWorker("go-realtime-search-sync")
//
// After Init, expose /metrics + /healthz + /readyz on a sidecar port:
//
//	go func() { _ = metrics.Serve(":9090") }()
//
// Instrumentation helpers — wrap each external call with the matching helper.
// All helpers record into shared metrics with consistent labels so the
// service map / dashboards / alerts work uniformly across all 14 services.
//
//	metrics.GinMiddleware()                                   // HTTP server
//	metrics.ObserveDownstream("auth-svc", fn)                 // service→service HTTP
//	metrics.ObserveDB("posts", "find_one", fn)                // service→DocDB
//	metrics.ObserveCache("listing", fn)                       // service→Redis/Valkey
//	metrics.ObserveExternal("s3", "put_object", fn)           // service→AWS / 3rd-party
//	metrics.ObserveJob("rank-update", fn)                     // worker job
//	metrics.RecordSync("posts", "meilisearch", "insert", n)   // CDC row count
//	metrics.SetSyncLag("posts", lagSec)                       // CDC lag
//	metrics.SetSyncProgress("posts", "initial_scan", 0.42)    // initial sync %
//	metrics.SetQueueDepth("approval", n)                      // backlog gauge
//
// Service-specific custom metrics still register on the exported Registry:
//
//	myBusinessCounter := prometheus.NewCounter(...)
//	metrics.Registry.MustRegister(myBusinessCounter)
package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServiceType — one of "bff", "system", "worker". Set by InitBFF/System/Worker
// and stamped as a const label on every standard metric so dashboards can
// filter by tier without per-service config.
type ServiceType string

const (
	TypeBFF    ServiceType = "bff"
	TypeSystem ServiceType = "system"
	TypeWorker ServiceType = "worker"
)

// Registry is the package-level Prometheus registry. Init* seeds it with
// default collectors (Go runtime + process) and the standard metric set
// appropriate for the chosen ServiceType. Services that need custom
// business metrics register them on this Registry too.
var Registry = prometheus.NewRegistry()

var (
	serviceName string
	serviceType ServiceType
	initOnce    sync.Once
	startTime   = time.Now()
)

// InitBFF prepares the registry for a BFF (Gin HTTP server + outbound HTTP
// to downstream services). Call once at service start, before any helper
// or middleware is used.
func InitBFF(name string) { initWith(name, TypeBFF) }

// InitSystem prepares the registry for a system service (Gin HTTP server +
// DocDB + Redis + external APIs).
func InitSystem(name string) { initWith(name, TypeSystem) }

// InitWorker prepares the registry for a worker / job runner (no HTTP server
// by default — register jobs via ObserveJob/RecordSync/etc.).
func InitWorker(name string) { initWith(name, TypeWorker) }

func initWith(name string, kind ServiceType) {
	initOnce.Do(func() {
		serviceName = name
		serviceType = kind
		startTime = time.Now()

		// Built-in collectors — Go runtime (goroutines, GC, memstats) +
		// process (RSS, CPU seconds, open FDs).
		Registry.MustRegister(collectors.NewGoCollector())
		Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

		registerHTTPServerMetrics()
		registerHTTPClientMetrics()
		registerDBMetrics()
		registerCacheMetrics()
		registerExternalMetrics()
		registerJobMetrics()
		registerSyncMetrics()
		registerQueueMetrics()
		registerHealthMetrics()
		registerBuildInfoMetrics()
	})
}

// ServiceName returns the name passed to Init*.
func ServiceName() string { return serviceName }

// Type returns the service type set by Init*.
func Type() ServiceType { return serviceType }

// constLabels returns the labels stamped on every standard metric.
// Service-specific custom metrics should also add these where useful.
func constLabels() prometheus.Labels {
	return prometheus.Labels{
		"service":      serviceName,
		"service_type": string(serviceType),
	}
}

// Serve starts an HTTP server on addr exposing:
//
//	/metrics — Prometheus scrape target
//	/healthz — liveness probe (200 unless registered checks all fail)
//	/readyz  — readiness probe (200 only when all readiness checks pass)
//
// Blocking — run in a goroutine. The metrics port is intentionally separate
// from the main application port so auth/rate-limit middleware on the main
// engine doesn't block scrapes or k8s probes.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		Registry:          Registry,
		EnableOpenMetrics: false,
	}))
	mux.HandleFunc("/healthz", livenessHandler)
	mux.HandleFunc("/readyz", readinessHandler)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

// resultLabel returns "ok" when err is nil, else "error". Used by all helpers
// to keep edge-success/edge-error queries uniform across metrics.
func resultLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
