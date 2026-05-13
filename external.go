package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	externalRequests *prometheus.CounterVec
	externalDuration *prometheus.HistogramVec
)

func registerExternalMetrics() {
	externalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nayoo_external_requests_total",
			Help: "Calls to external/AWS/3rd-party APIs (S3, SES, Keycloak, Meilisearch, etc.).",
		},
		[]string{"service", "service_type", "target", "op", "result"},
	)
	externalDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_external_duration_seconds",
			Help: "External API call latency in seconds.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1,
				0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0,
			},
		},
		[]string{"service", "service_type", "target", "op"},
	)
	Registry.MustRegister(externalRequests, externalDuration)
}

// ObserveExternal records latency + result of a call to an external system
// that isn't another NaYoo service and isn't DocDB / Redis.
//
// `target` — canonical name of the external system. Use these standard values
// so the service-map node graph aggregates correctly:
//
//	"s3", "ses", "keycloak", "meilisearch", "kinesis", "imgproxy",
//	"line", "discord", "slack", "external-api-<provider>"
//
// `op` — short verb describing the operation (e.g. "put_object", "send_email",
// "validate_token", "search", "publish").
//
//	err := metrics.ObserveExternal("s3", "put_object", func() error {
//	    _, err := s3Client.PutObject(ctx, input)
//	    return err
//	})
func ObserveExternal(target, op string, fn func() error) error {
	start := time.Now()
	err := fn()
	externalDuration.WithLabelValues(
		serviceName, string(serviceType), target, op,
	).Observe(time.Since(start).Seconds())
	externalRequests.WithLabelValues(
		serviceName, string(serviceType), target, op, resultLabel(err),
	).Inc()
	return err
}
