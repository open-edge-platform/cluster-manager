// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"slices"
)

// ProjectIDValidator validates the project ID in the request header
// It returns a Bad Request error if the project ID is missing or invalid
func ProjectIDValidator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip endpoints that do not require a project id
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
