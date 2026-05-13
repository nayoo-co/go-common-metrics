package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	cacheLookups  *prometheus.CounterVec
	cacheDuration *prometheus.HistogramVec
)

func registerCacheMetrics() {
	cacheLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_cache_lookups_total",
			Help: "Redis/Valkey cache lookups, labeled by key prefix and outcome.",
		},
		[]string{"service", "service_type", "target", "prefix", "result"},
	)
	cacheDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_cache_op_duration_seconds",
			Help: "Cache operation latency in seconds.",
			Buckets: []float64{
				0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5,
			},
		},
		[]string{"service", "service_type", "target", "prefix", "op"},
	)
	Registry.MustRegister(cacheLookups, cacheDuration)
}

// ObserveCache wraps a cache GET-style lookup.
//
// `prefix` — the logical key namespace (e.g., "listing", "user", "redirect").
// Avoid putting actual keys here — keep cardinality bounded.
//
// The callback returns (hit, err). `result` label is:
//   - "hit"   — found
//   - "miss"  — not found, no error
//   - "error" — call failed (connection / timeout)
//
// Target is hard-coded to "redis" (covers both ElastiCache Redis & Valkey).
//
//	val, hit := metrics.ObserveCache("listing", func() (string, bool) {
//	    v, err := rdb.Get(ctx, "listing:abc").Result()
//	    return v, err == nil
//	})
func ObserveCache(prefix string, fn func() (value any, hit bool, err error)) (any, bool, error) {
	start := time.Now()
	val, hit, err := fn()
	elapsed := time.Since(start).Seconds()

	result := "miss"
	switch {
	case err != nil:
		result = "error"
	case hit:
		result = "hit"
	}
	cacheDuration.WithLabelValues(
		serviceName, string(serviceType), "redis", prefix, "get",
	).Observe(elapsed)
	cacheLookups.WithLabelValues(
		serviceName, string(serviceType), "redis", prefix, result,
	).Inc()
	return val, hit, err
}

// ObserveCacheOp wraps any cache operation that isn't a GET — SET, DEL, EXPIRE etc.
// Use this when you don't have a hit/miss concept.
//
//	err := metrics.ObserveCacheOp("listing", "set", func() error {
//	    return rdb.Set(ctx, "listing:abc", val, 5*time.Minute).Err()
//	})
func ObserveCacheOp(prefix, op string, fn func() error) error {
	start := time.Now()
	err := fn()
	cacheDuration.WithLabelValues(
		serviceName, string(serviceType), "redis", prefix, op,
	).Observe(time.Since(start).Seconds())
	cacheLookups.WithLabelValues(
		serviceName, string(serviceType), "redis", prefix, resultLabel(err),
	).Inc()
	return err
}
