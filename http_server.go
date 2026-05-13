package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInflight *prometheus.GaugeVec
)

func registerHTTPServerMetrics() {
	if serviceType == TypeWorker {
		return // workers don't have HTTP server by default
	}
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_http_requests_total",
			Help: "Total HTTP requests handled, by method, route template, and status code.",
		},
		[]string{"service", "service_type", "method", "path", "code", "result"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_http_request_duration_seconds",
			Help: "HTTP request handling latency in seconds.",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1,
				0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
			},
		},
		[]string{"service", "service_type", "method", "path"},
	)
	httpInflight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_http_inflight_requests",
			Help: "HTTP requests currently being handled.",
		},
		[]string{"service", "service_type"},
	)
	Registry.MustRegister(httpRequests, httpDuration, httpInflight)
}

// GinMiddleware observes every HTTP request:
//   - increments nayoo_http_requests_total
//   - observes nayoo_http_request_duration_seconds
//   - tracks nayoo_http_inflight_requests (gauge inc/dec)
//
// The `path` label uses c.FullPath() (route template) — keeps cardinality
// bounded. Unmatched routes get path="<unmatched>".
// The `result` label is "ok" for 1xx/2xx/3xx, "error" for 4xx/5xx — gives
// a uniform success/error indicator across all standard metrics for the
// service-map dashboard.
//
// Init* must be called before this middleware is attached.
func GinMiddleware() gin.HandlerFunc {
	if httpRequests == nil {
		panic("metrics: GinMiddleware() requires Init{BFF,System} to be called first " +
			"(workers have no HTTP server middleware)")
	}
	return func(c *gin.Context) {
		httpInflight.WithLabelValues(serviceName, string(serviceType)).Inc()
		start := time.Now()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "<unmatched>"
		}
		status := c.Writer.Status()
		result := "ok"
		if status >= 400 {
			result = "error"
		}

		httpRequests.WithLabelValues(
			serviceName, string(serviceType),
			c.Request.Method, path, strconv.Itoa(status), result,
		).Inc()
		httpDuration.WithLabelValues(
			serviceName, string(serviceType), c.Request.Method, path,
		).Observe(time.Since(start).Seconds())
		httpInflight.WithLabelValues(serviceName, string(serviceType)).Dec()
	}
}
