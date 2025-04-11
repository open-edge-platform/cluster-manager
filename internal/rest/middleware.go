// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/metrics"
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
		if r.URL.Path != "/v2/healthz" { // reduce log spam
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
		next.ServeHTTP(w, r)
		metrics.HttpResponseCounter.WithLabelValues(r.Method, r.URL.Path, http.StatusText(http.StatusOK)).Inc()
	})
}

// projectIDValidator validates the project ID
func projectIDValidator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ignore /v2/healthz endpoint as it doesn't require project ID
		if r.URL.Path != "/v2/healthz" {
			activeProjectId := r.Header.Get("Activeprojectid")
			if activeProjectId == "" || activeProjectId == "00000000-0000-0000-0000-000000000000" {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"message": "no active project id provided"}`, http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
