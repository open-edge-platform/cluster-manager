// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"log/slog"
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

// appendMiddlewares returns a new handler with the provided middleware, executed in order
func appendMiddlewares(mw ...middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// logger logs the request and response
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(ignoredPaths, r.URL.Path) { // reduce log spam
			slog.Debug("received request", "method", r.Method, "path", r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func requestDurationMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start).Seconds()
		metrics.ResponseTime.Observe(d)
	})
}

func responseCounterMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// skip endpoints that do not require a project id
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		srw := metrics.NewStatusResponseWriter(w)
		next.ServeHTTP(srw, r)
		metrics.HttpResponseCounter.WithLabelValues(r.Method, r.URL.Path, srw.Status()).Inc()
	})
}

// projectIDValidator validates the project ID
func projectIDValidator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// skip endpoints that do not require a project id
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		activeProjectId := r.Header.Get("Activeprojectid")
		if activeProjectId == "" || activeProjectId == "00000000-0000-0000-0000-000000000000" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"message": "no active project id provided"}`, http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}
