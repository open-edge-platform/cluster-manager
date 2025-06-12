// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"log/slog"
	"net/http"
	"slices"
)

// Logger logs the request and response
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(ignoredPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		slog.Debug("received request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
