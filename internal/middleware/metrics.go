// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RequestDurationMetrics measures the duration of the request and records it for Prometheus
func RequestDurationMetrics(duration prometheus.Histogram, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start).Seconds()
		duration.Observe(d)
	})
}

// ResponseCounterMetrics counts the number of responses and records it for Prometheus
func ResponseCounterMetrics(counter *prometheus.CounterVec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		srw := newStatusResponseWriter(w)
		next.ServeHTTP(srw, r)
		counter.WithLabelValues(r.Method, r.URL.Path, srw.Status()).Inc()
	})
}

type statusResponseWriter struct {
	wr     http.ResponseWriter
	status int
}

func newStatusResponseWriter(wr http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{
		wr: wr,
	}
}

func (srw *statusResponseWriter) WriteHeader(code int) {
	srw.status = code
	srw.wr.WriteHeader(code)
}

func (srw *statusResponseWriter) Write(b []byte) (int, error) {
	return srw.wr.Write(b)
}

func (srw *statusResponseWriter) Header() http.Header {
	return srw.wr.Header()
}

func (srw *statusResponseWriter) Status() string {
	return strconv.Itoa(srw.status)
}
