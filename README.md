# go-common-metrics

Shared Prometheus client wrapper for all NaYoo Go services. Standardises the metric naming, port, and Gin middleware so the central Prometheus stack can scrape every service uniformly.

- **GitHub** (Go imports here): https://github.com/serpentdark/go-common-metrics
- **GitLab** (push here): https://gitlab.com/zero-to-one/nayoo/utils/go-common-metrics

GitLab is source of truth. Push to GitLab → push-mirror auto-syncs to GitHub → `go get github.com/serpentdark/go-common-metrics@vX.Y.Z` works.

## Installation

```bash
go get github.com/serpentdark/go-common-metrics@latest
```

## Standard usage (Gin HTTP service)

```go
package main

import (
    "os"

    "github.com/gin-gonic/gin"
    metrics "github.com/serpentdark/go-common-metrics"
)

func main() {
    metrics.Init("go-system-listing-service")
    go func() { _ = metrics.Serve(":9090") }()  // /metrics on sidecar port

    r := gin.Default()
    r.Use(metrics.GinMiddleware())              // observe every request

    // ... your routes ...

    _ = r.Run(":" + os.Getenv("PORT"))
}
```

## Worker (no Gin)

```go
metrics.Init("go-worker-listing-service")
go func() { _ = metrics.Serve(":9090") }()
// ... worker loop ...
```

## What's exposed at `:9090/metrics`

| Metric | Type | Labels | What |
| --- | --- | --- | --- |
| `nayoo_http_requests_total` | counter | service, method, path, code | request count per route + status |
| `nayoo_http_request_duration_seconds` | histogram | service, method, path | request latency |
| `nayoo_http_inflight_requests` | gauge | service | concurrent in-progress requests |
| `go_goroutines` | gauge | — | goroutine count (leak detection) |
| `go_memstats_*` | gauge | — | heap allocation / GC stats |
| `process_resident_memory_bytes` | gauge | — | pod RSS |
| `process_cpu_seconds_total` | counter | — | actual CPU consumed |
| `process_open_fds` | gauge | — | open file descriptors |

The `path` label uses Gin's `c.FullPath()` (route template like `/users/:id`) not raw URL — keeps cardinality bounded. Routes that don't match any handler get path label `<unmatched>`.

## Custom business metrics

Register your own collector on the exported `Registry`:

```go
docdbQueryDuration := prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "nayoo_docdb_query_duration_seconds",
        Help: "DocDB query latency in this service.",
    },
    []string{"service", "collection", "op"},
)
metrics.Registry.MustRegister(docdbQueryDuration)

// usage:
start := time.Now()
err := coll.FindOne(ctx, filter).Decode(&out)
docdbQueryDuration.
    WithLabelValues(metrics.ServiceName(), "products", "find_one").
    Observe(time.Since(start).Seconds())
```

## Helm chart wiring

Service chart needs to expose port 9090 so the cluster's PodMonitor can scrape it:

```yaml
ports:
  - name: http
    containerPort: 8080
  - name: metrics       # ← add this
    containerPort: 9090
```

The NaYoo monitoring stack has a single `PodMonitor` in `monitoring` ns that selects all pods with `prometheus.io/scrape: "true"` annotation OR matches `app.kubernetes.io/managed-by: nayoo-shared-lib`. See `helm-templates/grafana-prometheus-stack/` for details.

## Why port 9090 (sidecar style)

We mount metrics on a **separate** HTTP server, not on the main Gin engine, because:

1. The main engine's auth middleware would block scrapes.
2. Pod-level scrape doesn't need to hit the main port (which may have its own rate limits, TLS, etc).
3. Prometheus best practice — separation of concerns.

## Versioning

Semver. Cut a tag (`v1.0.0`, `v1.1.0`, ...) on GitLab `main` — push-mirror propagates to GitHub within ~10s. Consuming services bump via:

```bash
go get github.com/serpentdark/go-common-metrics@v1.1.0
go mod tidy
```
