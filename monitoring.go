package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	TypeHttp = "http"
	TypeFpm  = "fpm"
)

var (
	buckets = []float64{0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.000, 2.500, 5.000, 10.000}
)

type Monitor struct {
	Registry *prometheus.Registry

	HttpDurationHistogram *prometheus.HistogramVec
	FmpDurationHistogram  *prometheus.HistogramVec
}

func NewMonitor(logger *logrus.Logger) *Monitor {
	reg := prometheus.NewRegistry()
	monitor := &Monitor{
		Registry: reg,

		HttpDurationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of the complete request",
			Buckets: buckets,
		}, []string{"app", "type", "method", "http_code", "endpoint"}),
		FmpDurationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "phpfpm_request_duration_seconds",
			Help:    "Duration of the php fpm request",
			Buckets: buckets,
		}, []string{"app", "type", "method", "fpm_code", "endpoint"}),
	}

	reg.MustRegister(monitor.HttpDurationHistogram)
	reg.MustRegister(monitor.FmpDurationHistogram)

	logger.Debugf("Monitor initialized")

	return monitor
}
