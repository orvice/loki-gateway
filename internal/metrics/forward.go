package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	initOnce sync.Once

	attempt *prometheus.CounterVec
	success *prometheus.CounterVec
	fail    *prometheus.CounterVec
	latency *prometheus.HistogramVec
)

func initMetrics() {
	attempt = getOrRegisterCounterVec(prometheus.CounterOpts{
		Name: "forward_attempt_total",
		Help: "Total number of downstream forward attempts.",
	}, []string{"target", "endpoint"})

	success = getOrRegisterCounterVec(prometheus.CounterOpts{
		Name: "forward_success_total",
		Help: "Total number of successful downstream forwards.",
	}, []string{"target", "endpoint"})

	fail = getOrRegisterCounterVec(prometheus.CounterOpts{
		Name: "forward_fail_total",
		Help: "Total number of failed downstream forwards.",
	}, []string{"target", "endpoint", "reason"})

	latency = getOrRegisterHistogramVec(prometheus.HistogramOpts{
		Name:    "forward_latency_seconds",
		Help:    "Latency of downstream forward requests.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"target", "endpoint"})
}

func getOrRegisterCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	collector := prometheus.NewCounterVec(opts, labelNames)
	if err := prometheus.Register(collector); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return collector
}

func getOrRegisterHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	collector := prometheus.NewHistogramVec(opts, labelNames)
	if err := prometheus.Register(collector); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}
	}
	return collector
}

func RecordAttempt(target, endpoint string) {
	initOnce.Do(initMetrics)
	attempt.WithLabelValues(target, endpoint).Inc()
}

func RecordSuccess(target, endpoint string) {
	initOnce.Do(initMetrics)
	success.WithLabelValues(target, endpoint).Inc()
}

func RecordFail(target, endpoint, reason string) {
	initOnce.Do(initMetrics)
	if reason == "" {
		reason = "unknown"
	}
	fail.WithLabelValues(target, endpoint, reason).Inc()
}

func RecordLatency(target, endpoint string, seconds float64) {
	initOnce.Do(initMetrics)
	latency.WithLabelValues(target, endpoint).Observe(seconds)
}
