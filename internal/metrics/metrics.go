// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ResponseTime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "cluster_manager_http_response_time_seconds_histogram",
		Help:    "Response time to HTTP requests in seconds",
		Buckets: prometheus.DefBuckets,
	})

	HttpResponseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cluster_manager_http_response_codes_counter",
			Help: "Count of HTTP response codes per endpoint",
		},
		[]string{"method", "path", "code"},
	)
)

func GetRegistry() *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(ResponseTime)
	registry.MustRegister(HttpResponseCounter)

	return registry
}
