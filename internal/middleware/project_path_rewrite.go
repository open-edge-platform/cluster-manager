// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"regexp"
)

var projectScopedPathRegex = regexp.MustCompile(`^(/v[0-9]+)/projects/[^/]+(/.*)$`)

// RewriteProjectScopedPath maps /vN/projects/{projectName}/... requests onto the
// existing /vN/... API surface after project resolution has already happened.
func RewriteProjectScopedPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matches := projectScopedPathRegex.FindStringSubmatch(r.URL.Path)
		if len(matches) == 3 {
			r.URL.Path = matches[1] + matches[2]
		}

		next.ServeHTTP(w, r)
	})
}
