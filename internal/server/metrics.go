package server

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	cacheHits     prometheus.Counter
	cacheMisses   prometheus.Counter
	cacheStales   prometheus.Counter
	originErrors  prometheus.Counter
	originLatency prometheus.Histogram
	bytesServed   prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		cacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "proxy",
			Name:      "cache_hits_total",
			Help:      "Number of cache hits",
		}),
		cacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "proxy",
			Name:      "cache_misses_total",
			Help:      "Number of cache misses",
		}),
		cacheStales: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "proxy",
			Name:      "cache_stale_total",
			Help:      "Number of stale cache reuses",
		}),
		originErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "proxy",
			Name:      "origin_errors_total",
			Help:      "Number of origin errors",
		}),
		originLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "proxy",
			Name:      "origin_latency_seconds",
			Help:      "Latency of origin fetches",
			Buckets:   prometheus.DefBuckets,
		}),
		bytesServed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "proxy",
			Name:      "bytes_served_total",
			Help:      "Total bytes served to clients",
		}),
	}

	reg.MustRegister(m.cacheHits, m.cacheMisses, m.cacheStales, m.originErrors, m.originLatency, m.bytesServed)
	return m
}
