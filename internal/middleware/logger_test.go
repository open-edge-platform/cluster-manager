// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name     string
		path     string
		method   string
		expected string
	}{
		{
			name:     "GET request",
			path:     "/v2/clusters",
			method:   "GET",
			expected: "method=GET path=/v2/clusters",
		},
		{
			name:     "Ignored halthz path",
			path:     "/v2/healthz",
			method:   "GET",
			expected: "", // No logging expected
		},
		{
			name:     "Ignored metrics path",
			path:     "/metrics",
			method:   "GET",
			expected: "", // No logging expected
		},
		{
			name:     "POST request",
			path:     "/v2/templates",
			method:   "POST",
			expected: "method=POST path=/v2/templates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var logBuf bytes.Buffer
			handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := slog.New(handler)

			// Replace the default logger
			oldLogger := slog.Default()
			slog.SetDefault(logger)
			defer slog.SetDefault(oldLogger)

			// Create a request
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Create a handler to check if it's called
			handlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			// Apply the middleware
			middlewareFunc := Logger(nextHandler)

			// Serve the request
			middlewareFunc.ServeHTTP(rr, req)

			// Verify the handler was called
			if !handlerCalled {
				t.Error("Next handler was not called")
			}

			// Check the log output
			logOutput := logBuf.String()
			if tt.expected == "" {
				if strings.Contains(logOutput, "received request") {
					t.Errorf("Expected no logging for ignored path, got %q", logOutput)
				}
			} else {
				if !strings.Contains(logOutput, tt.expected) {
					t.Errorf("Expected log output containing %q, got %q", tt.expected, logOutput)
				}
				if !strings.Contains(logOutput, "received request") {
					t.Errorf("Log message doesn't contain 'received request': %q", logOutput)
				}
			}

			// Check response status
			if status := rr.Code; status != http.StatusOK {
				t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
			}
		})
	}
}
