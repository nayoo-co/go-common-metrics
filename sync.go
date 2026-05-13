package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	syncRows     *prometheus.CounterVec
	syncBatch    *prometheus.HistogramVec
	syncLag      *prometheus.GaugeVec
	syncProgress *prometheus.GaugeVec
)

func registerSyncMetrics() {
	if serviceType != TypeWorker {
		return // sync metrics meaningful only for CDC/sync workers
	}
	syncRows = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_sync_rows_total",
			Help: "Total rows transferred between source and target. Use op=insert/update/delete for CDC stream, op=initial for backfill.",
		},
		[]string{"service", "service_type", "source", "target", "op", "result"},
	)
	syncBatch = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_sync_batch_duration_seconds",
			Help: "Sync batch processing latency in seconds.",
			Buckets: []float64{
				0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
			},
		},
		[]string{"service", "service_type", "source", "target"},
	)
	syncLag = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_sync_lag_seconds",
			Help: "Age of the most recently synced record at source (now - source_event_time).",
		},
		[]string{"service", "service_type", "source"},
	)
	syncProgress = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_sync_progress_ratio",
			Help: "Progress of the named phase, 0.0–1.0. Use phase=initial_scan for backfill, phase=catchup for stream replay.",
		},
		[]string{"service", "service_type", "source", "phase"},
	)
	Registry.MustRegister(syncRows, syncBatch, syncLag, syncProgress)
}

// RecordSync increments the row counter for a sync operation. Call from
// CDC handlers / initial scan loops after each batch.
//
// `source` — origin collection / table / topic name (e.g., "posts").
// `target` — sink system / index (e.g., "meilisearch", "valkey", "downstream-api").
// `op`     — one of: initial / insert / update / delete / upsert.
// `rows`   — count of rows in this batch (use 1 for single-row events).
// `err`    — nil → result=ok, otherwise result=error.
//
//	metrics.RecordSync("posts", "meilisearch", "insert", 250, nil)
func RecordSync(source, target, op string, rows int, err error) {
	if syncRows == nil {
		return
	}
	syncRows.WithLabelValues(
		serviceName, string(serviceType), source, target, op, resultLabel(err),
	).Add(float64(rows))
}

// ObserveSyncBatch wraps a batch processing function — records rows + duration
// in one call. Use this when you have a batch you can wrap; otherwise call
// RecordSync directly.
//
//	rows, err := metrics.ObserveSyncBatch("posts", "meilisearch", func() (int, error) {
//	    return indexBatch(docs)
//	})
func ObserveSyncBatch(source, target string, fn func() (rows int, err error)) (int, error) {
	if syncBatch == nil {
		return fn()
	}
	start := time.Now()
	rows, err := fn()
	syncBatch.WithLabelValues(
		serviceName, string(serviceType), source, target,
	).Observe(time.Since(start).Seconds())
	op := "insert"
	if err != nil {
		op = "error"
	}
	RecordSync(source, target, op, rows, err)
	return rows, err
}

// SetSyncLag reports how stale the sync pipeline is for a given source.
// Compute as (now - eventTimeOfLastProcessedRecord).Seconds().
//
//	metrics.SetSyncLag("posts", time.Since(lastEventTime).Seconds())
func SetSyncLag(source string, seconds float64) {
	if syncLag == nil {
		return
	}
	syncLag.WithLabelValues(serviceName, string(serviceType), source).Set(seconds)
}

// SetSyncProgress reports phase-specific completion ratio 0.0–1.0.
// Typical phases: "initial_scan", "catchup".
//
//	metrics.SetSyncProgress("posts", "initial_scan", float64(processed)/float64(total))
func SetSyncProgress(source, phase string, ratio float64) {
	if syncProgress == nil {
		return
	}
	syncProgress.WithLabelValues(serviceName, string(serviceType), source, phase).Set(ratio)
}
