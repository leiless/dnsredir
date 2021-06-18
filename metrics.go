package dnsredir

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Time buckets used for name lookup duration in milliseconds
	nameLookupBuckets = []float64{
		1, 5, 10, 15, 20, 25, 30, 35, 40, 50, 60, 80, 100, 125, 150,
	}
	// This metric value mainly used for benchmarking purpose
	NameLookupDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "name_lookup_duration_ms",
		Buckets:   nameLookupBuckets,
		Help:      "Histogram of the time(in milliseconds) each name lookup took.",
	}, []string{"server", "matched"})

	requestBuckets = []float64{
		15, 30, 50, 75, 100, 200, 350, 500, 750, 1000, 2000, 4000, 8000,
	}
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "request_duration_ms",
		Buckets:   requestBuckets,
		Help:      "Histogram of the time(in milliseconds) each request took.",
	}, []string{"server", "to"})

	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "request_count_total",
		Help:      "Counter of requests made per upstream.",
	}, []string{"server", "to"})

	RcodeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "response_rcode_count_total",
		Help:      "Rcode counter of requests made per upstream.",
	}, []string{"server", "to", "rcode"})

	// XXX: currently server not embedded into hc failure count label
	HealthCheckFailureCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "hc_failure_count_total",
		Help:      "Counter of the number of failed healthchecks.",
	}, []string{"to"})

	// XXX: Ditto.
	HealthCheckAllDownCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "hc_all_down_count_total",
		Help:      "Counter of the number of complete failures of the healthchecks.",
	}, []string{"to"})
)
