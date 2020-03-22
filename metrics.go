package dnsredir

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	NameLookupDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name: "name_lookup_duration_ms",
		Buckets:   plugin.TimeBuckets,
		Help: "Histogram of the time(in milliseconds) each name lookup took.",
	}, []string{"status"})
)

