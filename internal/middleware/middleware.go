// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
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

// RequestDurationMetrics moved to metrics_middleware.go

// ResponseCounterMetrics moved to metrics_middleware.go

// ProjectIDValidator moved to project_validator.go
