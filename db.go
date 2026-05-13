package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	dbQueries  *prometheus.CounterVec
	dbDuration *prometheus.HistogramVec
)

func registerDBMetrics() {
	if serviceType == TypeBFF {
		return // BFF talks to system services, not DB directly
	}
	dbQueries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_db_queries_total",
			Help: "Total DocDB queries, labeled by collection, op, and result.",
		},
		[]string{"service", "service_type", "target", "collection", "op", "result"},
	)
	dbDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_db_query_duration_seconds",
			Help: "DocDB query latency in seconds.",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1,
				0.25, 0.5, 1.0, 2.5, 5.0,
			},
		},
		[]string{"service", "service_type", "target", "collection", "op"},
	)
	Registry.MustRegister(dbQueries, dbDuration)
}

// ObserveDB wraps a DocDB operation with timing + result.
//
// `collection` — DocDB collection name (e.g., "posts", "users")
// `op` — one of: find / find_one / insert / update / delete / aggregate / count
//
// The target label is hard-coded to "docdb" so the service-map node graph
// shows a single DocDB node aggregating queries from all services.
//
//	err := metrics.ObserveDB("posts", "find_one", func() error {
//	    return coll.FindOne(ctx, filter).Decode(&out)
//	})
func ObserveDB(collection, op string, fn func() error) error {
	if dbQueries == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	dbDuration.WithLabelValues(
		serviceName, string(serviceType), "docdb", collection, op,
	).Observe(time.Since(start).Seconds())
	dbQueries.WithLabelValues(
		serviceName, string(serviceType), "docdb", collection, op, resultLabel(err),
	).Inc()
	return err
}
