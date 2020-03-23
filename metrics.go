package dnsredir

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Time buckets used for name lookup duration in milliseconds
	nameLookupBuckets = []float64{
		1, 5, 10, 15, 20, 25, 30, 35, 40, 50, 60, 80, 100, 125, 150,
	}

	NameLookupDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name: "name_lookup_duration_ms",
		Buckets: nameLookupBuckets,
		Help: "Histogram of the time(in milliseconds) each name lookup took.",
	}, []string{"status"})
)

