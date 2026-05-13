package metrics

import "github.com/prometheus/client_golang/prometheus"

var queueDepth *prometheus.GaugeVec

func registerQueueMetrics() {
	queueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_queue_depth",
			Help: "Backlog size of a named in-process queue / pending-work counter.",
		},
		[]string{"service", "service_type", "queue"},
	)
	Registry.MustRegister(queueDepth)
}

// SetQueueDepth reports the current backlog of a named queue.
// Service maintains the gauge — call from wherever you enqueue/dequeue, or
// from a periodic goroutine that polls queue size.
//
//	metrics.SetQueueDepth("approval", float64(len(pending)))
//
// Examples of "queue" names: "approval", "notification", "task", "indexing".
func SetQueueDepth(name string, value float64) {
	if queueDepth == nil {
		return
	}
	queueDepth.WithLabelValues(serviceName, string(serviceType), name).Set(value)
}
