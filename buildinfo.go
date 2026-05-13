package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	buildInfo *prometheus.GaugeVec
	uptime    prometheus.CounterFunc
)

func registerBuildInfoMetrics() {
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nayoo_build_info",
			Help: "Service build/version metadata. Always set to 1 — use the labels for identity.",
		},
		[]string{"service", "service_type", "version", "git_sha", "build_time", "go_version"},
	)
	Registry.MustRegister(buildInfo)

	uptime = prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name:        "nayoo_uptime_seconds_total",
			Help:        "Seconds since this service process started.",
			ConstLabels: constLabels(),
		},
		func() float64 { return time.Since(startTime).Seconds() },
	)
	Registry.MustRegister(uptime)
}

// RegisterBuildInfo records build metadata as a constant gauge (= 1).
// Typically the values are injected at build time via -ldflags:
//
//	go build -ldflags "-X main.version=v1.2.3 -X main.gitSHA=abcd1234 -X main.buildTime=2026-05-13T07:00:00Z"
//
// then in main.go:
//
//	var (version, gitSHA, buildTime string)
//	metrics.RegisterBuildInfo(version, gitSHA, buildTime)
//
// goVersion is auto-filled from runtime.Version() if you pass "".
func RegisterBuildInfo(version, gitSHA, buildTime string) {
	if buildInfo == nil {
		return
	}
	if version == "" {
		version = "dev"
	}
	if gitSHA == "" {
		gitSHA = "unknown"
	}
	if buildTime == "" {
		buildTime = "unknown"
	}
	buildInfo.WithLabelValues(
		serviceName, string(serviceType), version, gitSHA, buildTime, runtimeVersion(),
	).Set(1)
}
