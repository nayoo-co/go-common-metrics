package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	downstreamRequests *prometheus.CounterVec
	downstreamDuration *prometheus.HistogramVec
)

func registerHTTPClientMetrics() {
	if serviceType == TypeWorker {
		return // workers usually don't call downstream HTTP services
	}
	downstreamRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_downstream_requests_total",
			Help: "Outbound HTTP requests to downstream services, by target service.",
		},
		[]string{"service", "service_type", "target", "code", "result"},
	)
	downstreamDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_downstream_duration_seconds",
			Help: "Outbound HTTP request latency in seconds.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1,
				0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
			},
		},
		[]string{"service", "service_type", "target"},
	)
	Registry.MustRegister(downstreamRequests, downstreamDuration)
}

// ObserveDownstream records latency + status of a downstream HTTP call.
//
// `target` should be the canonical caller-side name of the downstream service
// (e.g., "go-system-listing-service"). Keep it stable across services so the
// service-map node graph connects edges cleanly. If the call doesn't go
// over HTTP, prefer ObserveExternal instead.
//
// Pass a function that returns the HTTP status code (0 if the request never
// got a response — connection refused / timeout) plus an error. The helper
// derives `result` from the error.
//
//	statusCode, err := metrics.ObserveDownstream("auth-svc", func() (int, error) {
//	    resp, err := client.Get(url)
//	    if err != nil { return 0, err }
//	    defer resp.Body.Close()
//	    return resp.StatusCode, nil
//	})
func ObserveDownstream(target string, fn func() (int, error)) (int, error) {
	if downstreamRequests == nil {
		// silent no-op for worker type
		return fn()
	}
	start := time.Now()
	code, err := fn()
	downstreamDuration.WithLabelValues(
		serviceName, string(serviceType), target,
	).Observe(time.Since(start).Seconds())
	downstreamRequests.WithLabelValues(
		serviceName, string(serviceType), target,
		strconv.Itoa(code), resultLabel(err),
	).Inc()
	return code, err
}
