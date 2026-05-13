package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HealthCheck — a dependency probe. Return nil → healthy, error → unhealthy.
// Implementations should respect ctx and complete fast (< 1s typical).
type HealthCheck func(ctx context.Context) error

var (
	healthMu         sync.RWMutex
	livenessChecks   = map[string]HealthCheck{}
	readinessChecks  = map[string]HealthCheck{}
	healthCheckGauge *prometheus.GaugeVec
	healthCheckDur   *prometheus.HistogramVec
)

func registerHealthMetrics() {
	healthCheckGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_health_check",
			Help: "Status of a registered health check — 1 healthy, 0 failing.",
		},
		[]string{"service", "service_type", "kind", "check"},
	)
	healthCheckDur = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "nayoo_health_check_duration_seconds",
			Help: "Time taken by each health check probe.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0,
			},
		},
		[]string{"service", "service_type", "kind", "check"},
	)
	Registry.MustRegister(healthCheckGauge, healthCheckDur)
}

// RegisterLivenessCheck adds a probe to /healthz. Liveness checks should be
// SHALLOW — only fail if the process itself is broken (deadlock detector,
// internal kill-switch). Don't fail liveness on a downstream outage — that
// would cause unnecessary restarts.
func RegisterLivenessCheck(name string, check HealthCheck) {
	healthMu.Lock()
	livenessChecks[name] = check
	healthMu.Unlock()
}

// RegisterReadinessCheck adds a probe to /readyz. Readiness checks SHOULD
// fail when the service can't serve traffic — DocDB down, Redis unreachable,
// downstream service hard-required for this service to function, etc.
// k8s removes the pod from Service endpoints when /readyz returns 503.
func RegisterReadinessCheck(name string, check HealthCheck) {
	healthMu.Lock()
	readinessChecks[name] = check
	healthMu.Unlock()
}

// startHealthGaugeRefresher runs liveness + readiness checks on a 15s ticker
// so the nayoo_health_check gauge reflects current state regardless of whether
// /healthz or /readyz is being hit. k8s probes typically target the app's main
// HTTP port (8080) and never call our :9090 endpoints, so without this ticker
// the gauge would stay unset for the lifetime of the pod.
//
// Started once by initWith() in metrics.go. Each tick budgets 5s total across
// all checks; an individual check should still complete in < 1s.
func startHealthGaugeRefresher() {
	go func() {
		// First tick after a small delay so registered checks have a chance to
		// finish their async setup (e.g. mongo.Connect in main).
		time.Sleep(3 * time.Second)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = runChecks(ctx, "liveness", livenessChecks)
			_ = runChecks(ctx, "readiness", readinessChecks)
			cancel()
			<-ticker.C
		}
	}()
}

type checkReport struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type healthReport struct {
	Service     string        `json:"service"`
	ServiceType string        `json:"service_type"`
	Status      string        `json:"status"`
	Uptime      string        `json:"uptime"`
	Checks      []checkReport `json:"checks"`
}

func runChecks(ctx context.Context, kind string, checks map[string]HealthCheck) []checkReport {
	healthMu.RLock()
	snapshot := make(map[string]HealthCheck, len(checks))
	for k, v := range checks {
		snapshot[k] = v
	}
	healthMu.RUnlock()

	out := make([]checkReport, 0, len(snapshot))
	for name, check := range snapshot {
		start := time.Now()
		err := check(ctx)
		elapsed := time.Since(start).Seconds()

		status := "healthy"
		errStr := ""
		gaugeVal := 1.0
		if err != nil {
			status = "unhealthy"
			errStr = err.Error()
			gaugeVal = 0.0
		}
		if healthCheckGauge != nil {
			healthCheckGauge.WithLabelValues(
				serviceName, string(serviceType), kind, name,
			).Set(gaugeVal)
			healthCheckDur.WithLabelValues(
				serviceName, string(serviceType), kind, name,
			).Observe(elapsed)
		}
		out = append(out, checkReport{Name: name, Status: status, Error: errStr})
	}
	return out
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	reports := runChecks(ctx, "liveness", livenessChecks)

	status := "healthy"
	httpCode := http.StatusOK
	for _, rep := range reports {
		if rep.Status != "healthy" {
			status = "unhealthy"
			httpCode = http.StatusServiceUnavailable
			break
		}
	}
	writeHealth(w, httpCode, healthReport{
		Service:     serviceName,
		ServiceType: string(serviceType),
		Status:      status,
		Uptime:      time.Since(startTime).Truncate(time.Second).String(),
		Checks:      reports,
	})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	reports := runChecks(ctx, "readiness", readinessChecks)

	status := "ready"
	httpCode := http.StatusOK
	for _, rep := range reports {
		if rep.Status != "healthy" {
			status = "not_ready"
			httpCode = http.StatusServiceUnavailable
			break
		}
	}
	writeHealth(w, httpCode, healthReport{
		Service:     serviceName,
		ServiceType: string(serviceType),
		Status:      status,
		Uptime:      time.Since(startTime).Truncate(time.Second).String(),
		Checks:      reports,
	})
}

func writeHealth(w http.ResponseWriter, code int, report healthReport) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(report)
}
