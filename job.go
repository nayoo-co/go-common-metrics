package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobRuns     *prometheus.CounterVec
	jobDuration *prometheus.HistogramVec
)

func registerJobMetrics() {
	if serviceType != TypeWorker {
		return // jobs only meaningful for worker tier
	}
	jobRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_job_runs_total",
			Help: "Total background job invocations, by job name and result.",
		},
		[]string{"service", "service_type", "job", "result"},
	)
	jobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_job_duration_seconds",
			Help: "Background job execution latency in seconds.",
			Buckets: []float64{
				0.1, 0.5, 1, 5, 10, 30, 60, 300, 600, 1800, 3600,
			},
		},
		[]string{"service", "service_type", "job"},
	)
	Registry.MustRegister(jobRuns, jobDuration)
}

// ObserveJob wraps a background job invocation — records run count + duration.
//
// `name` — short stable identifier for the job (e.g., "ctr-update",
// "approve-posts", "sync-zones"). Keep cardinality bounded (don't include
// timestamps / IDs).
//
//	err := metrics.ObserveJob("ctr-update", func() error {
//	    return runCTRUpdate(ctx)
//	})
func ObserveJob(name string, fn func() error) error {
	if jobRuns == nil {
		return fn() // no-op on non-worker tiers
	}
	start := time.Now()
	err := fn()
	jobDuration.WithLabelValues(
		serviceName, string(serviceType), name,
	).Observe(time.Since(start).Seconds())
	jobRuns.WithLabelValues(
		serviceName, string(serviceType), name, resultLabel(err),
	).Inc()
	return err
}
