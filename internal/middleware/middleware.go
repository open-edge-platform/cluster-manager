// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"slices"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/metrics"
)

var (
	ignoredPaths = []string{
		"/v2/healthz",
		"/metrics",
	}
)

// middleware is a function definition that wraps an http.Handler
type middleware func(http.Handler) http.Handler

// Append returns a new handler with the provided middleware, executed in order
func Append(mw ...middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// Logger moved to logger.go

// RequestDurationMetrics measures the duration of the request and records it for Prometheus
func RequestDurationMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start).Seconds()
		metrics.ResponseTime.Observe(d)
	})
}

// ResponseCounterMetrics counts the number of responses and records it for Prometheus
func ResponseCounterMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		srw := metrics.NewStatusResponseWriter(w)
		next.ServeHTTP(srw, r)
		metrics.HttpResponseCounter.WithLabelValues(r.Method, r.URL.Path, srw.Status()).Inc()
	})
}

// ProjectIDValidator moved to project_validator.go
