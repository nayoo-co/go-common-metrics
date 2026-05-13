# go-common-metrics

Shared Prometheus client wrapper for every NaYoo Go service. One library, three entry points — pick the one that matches your service type — and the right standard metric set, helpers, health endpoints, and dashboards/alerts/service-map come for free.

- **GitHub** (Go imports here): https://github.com/serpentdark/go-common-metrics
- **GitLab** (push here): https://gitlab.com/zero-to-one/nayoo/utils/go-common-metrics

GitLab is source of truth. Push to GitLab → push-mirror auto-syncs to GitHub → `go get github.com/serpentdark/go-common-metrics@vX.Y.Z` works.

## Installation

```bash
go get github.com/serpentdark/go-common-metrics@latest
```

## Choose your entry point

| Tier | Init call | Default metric surface |
| --- | --- | --- |
| **BFF** (API gateway, Gin → downstream services) | `metrics.InitBFF("go-process-nayoo-service")` | HTTP server + downstream HTTP + cache + health + uptime |
| **System** (Gin + DocDB + Redis + AWS APIs) | `metrics.InitSystem("go-system-listing-service")` | All of BFF + DocDB + external + queue |
| **Worker** (no HTTP server by default) | `metrics.InitWorker("go-realtime-search-sync")` | DocDB + cache + external + job + sync + queue + health + uptime |

## Quick start (system service)

```go
package main

import (
    "context"
    "os"

    "github.com/gin-gonic/gin"
    metrics "github.com/serpentdark/go-common-metrics"
)

// injected via -ldflags at build time
var (version, gitSHA, buildTime string)

func main() {
    metrics.InitSystem("go-system-listing-service")
    metrics.RegisterBuildInfo(version, gitSHA, buildTime)

    // Optional: register dependency probes — show up at /readyz + as nayoo_health_check metric
    metrics.RegisterReadinessCheck("docdb",       func(ctx context.Context) error { return db.Ping(ctx, nil) })
    metrics.RegisterReadinessCheck("redis",       func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
    metrics.RegisterReadinessCheck("meilisearch", func(ctx context.Context) error { _, err := mc.Health(); return err })

    go func() { _ = metrics.Serve(":9090") }()

    r := gin.Default()
    r.Use(metrics.GinMiddleware())
    // ... routes ...
    _ = r.Run(":" + os.Getenv("PORT"))
}
```

## Worker (sync) example

```go
metrics.InitWorker("go-realtime-search-sync")
metrics.RegisterBuildInfo(version, gitSHA, buildTime)
metrics.RegisterReadinessCheck("docdb",       docdbCheck)
metrics.RegisterReadinessCheck("meilisearch", meiliCheck)
go func() { _ = metrics.Serve(":9090") }()

// initial scan
for batch := range scanBatches {
    rows, err := metrics.ObserveSyncBatch("posts", "meilisearch", func() (int, error) {
        return indexBatch(batch)
    })
    metrics.SetSyncProgress("posts", "initial_scan",
        float64(processed+rows)/float64(totalDocs))
    processed += rows
    _ = err
}

// change-stream loop
for evt := range stream {
    metrics.RecordSync("posts", "meilisearch", strings.ToLower(evt.Op), 1,
        applyEvent(evt))
    metrics.SetSyncLag("posts", time.Since(evt.ClusterTime).Seconds())
}
```

## Instrumentation helpers — call patterns

| Helper | Signature (short) | Metric written |
| --- | --- | --- |
| `GinMiddleware()` | `gin.HandlerFunc` | `nayoo_http_*` (BFF + system) |
| `ObserveDownstream(target, fn)` | `(int, error)` | `nayoo_downstream_*` (BFF + system) |
| `ObserveDB(coll, op, fn)` | `error` | `nayoo_db_*` (system + worker) |
| `ObserveCache(prefix, fn)` | `(value, hit, err)` | `nayoo_cache_*` |
| `ObserveCacheOp(prefix, op, fn)` | `error` | `nayoo_cache_*` |
| `ObserveExternal(target, op, fn)` | `error` | `nayoo_external_*` |
| `ObserveJob(name, fn)` | `error` (worker only) | `nayoo_job_*` |
| `RecordSync(src, tgt, op, n, err)` | — (worker only) | `nayoo_sync_rows_total` |
| `ObserveSyncBatch(src, tgt, fn)` | `(int, error)` | `nayoo_sync_batch_duration_seconds` + rows |
| `SetSyncLag(src, sec)` | — | `nayoo_sync_lag_seconds` |
| `SetSyncProgress(src, phase, ratio)` | — | `nayoo_sync_progress_ratio` |
| `SetQueueDepth(name, n)` | — | `nayoo_queue_depth` |
| `RegisterLivenessCheck(name, fn)` | — | `/healthz` + `nayoo_health_check{kind=liveness}` |
| `RegisterReadinessCheck(name, fn)` | — | `/readyz` + `nayoo_health_check{kind=readiness}` |
| `RegisterBuildInfo(ver, sha, time)` | — | `nayoo_build_info` |

## Standard metric inventory

Every metric carries `service` + `service_type` labels (`bff` / `system` / `worker`) plus the metric-specific labels below.

| Metric | Type | Specific labels |
| --- | --- | --- |
| `nayoo_http_requests_total` | counter | method, path, code, result |
| `nayoo_http_request_duration_seconds` | histogram | method, path |
| `nayoo_http_inflight_requests` | gauge | — |
| `nayoo_downstream_requests_total` | counter | target, code, result |
| `nayoo_downstream_duration_seconds` | histogram | target |
| `nayoo_db_queries_total` | counter | target=docdb, collection, op, result |
| `nayoo_db_query_duration_seconds` | histogram | target=docdb, collection, op |
| `nayoo_cache_lookups_total` | counter | target=redis, prefix, result |
| `nayoo_cache_op_duration_seconds` | histogram | target=redis, prefix, op |
| `nayoo_external_requests_total` | counter | target, op, result |
| `nayoo_external_duration_seconds` | histogram | target, op |
| `nayoo_job_runs_total` | counter | job, result |
| `nayoo_job_duration_seconds` | histogram | job |
| `nayoo_sync_rows_total` | counter | source, target, op, result |
| `nayoo_sync_batch_duration_seconds` | histogram | source, target |
| `nayoo_sync_lag_seconds` | gauge | source |
| `nayoo_sync_progress_ratio` | gauge | source, phase |
| `nayoo_queue_depth` | gauge | queue |
| `nayoo_health_check` | gauge (0/1) | kind, check |
| `nayoo_health_check_duration_seconds` | histogram | kind, check |
| `nayoo_build_info` | gauge=1 | version, git_sha, build_time, go_version |
| `nayoo_uptime_seconds_total` | counter | — |
| `go_*`, `process_*` | (built-in) | — |

## Service-specific business metrics

Register your own on the exported `Registry`:

```go
listingsCreated := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "listing_created_total",
        Help: "Listings created by users.",
    },
    []string{"service", "service_type", "province", "category"},
)
metrics.Registry.MustRegister(listingsCreated)

// usage:
listingsCreated.WithLabelValues(
    metrics.ServiceName(), string(metrics.Type()),
    listing.Province, listing.Category,
).Inc()
```

**Cardinality rule**: every label-value combination becomes a new time series. Don't use user/listing/session IDs, raw URLs, emails, etc. as labels. Stick to enumerable values: methods, route templates, status codes, provinces (77 in TH), categories (~10), op names.

## Service map / Node graph

Every helper writes a uniform `target` label so a Grafana Node Graph panel (provided as `dashboards/aws-service-map.json` in `helm-templates/grafana-prometheus-stack/`) can derive the service-to-service / service-to-system topology from one PromQL query:

```promql
sum by (service, target) (
  rate(nayoo_downstream_requests_total[5m])
  or rate(nayoo_db_queries_total[5m])
  or rate(nayoo_cache_lookups_total[5m])
  or rate(nayoo_external_requests_total[5m])
)
```

## Helm chart wiring

Expose port 9090 in your service Deployment template:

```yaml
ports:
  - name: http
    containerPort: 8080
  - name: metrics
    containerPort: 9090
livenessProbe:
  httpGet: { path: /healthz, port: metrics }
  initialDelaySeconds: 30
  periodSeconds: 30
readinessProbe:
  httpGet: { path: /readyz, port: metrics }
  initialDelaySeconds: 10
  periodSeconds: 10
```

A single `PodMonitor` in the `monitoring` ns (deployed by `helm-templates/grafana-prometheus-stack/`) discovers any pod that has a port named `metrics` and an annotation `prometheus.io/scrape: "true"`.

## Versioning

Semver. Cut a tag (`v1.0.0`, `v1.1.0`, …) on GitLab `main` — push-mirror propagates to GitHub within ~10s. Consuming services bump via:

```bash
go get github.com/serpentdark/go-common-metrics@v1.1.0
go mod tidy
```
