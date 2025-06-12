// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// prometheus.Hisogram stub
type stubHistogram struct {
	prometheus.Histogram
	value float64
}

func (s *stubHistogram) Observe(v float64) {
	s.value = v
}

func TestRequestDurationMetrics(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wait         time.Duration
		shouldRecord bool
	}{
		{
			name:         "Standard path",
			path:         "/v2/clusters",
			wait:         10 * time.Millisecond,
			shouldRecord: true,
		},
		{
			name:         "Ignored path",
			path:         "/v2/healthz",
			wait:         10 * time.Millisecond,
			shouldRecord: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Create a handler with a delay to test timing
			handlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				time.Sleep(tt.wait)
				w.WriteHeader(http.StatusOK)
			})

			// Create a stub histogram for testing
			histogram := &stubHistogram{}

			// Apply the middleware
			middlewareFunc := RequestDurationMetrics(histogram, nextHandler)

			// Serve the request
			middlewareFunc.ServeHTTP(rr, req)

			// Verify the handler was called
			if !handlerCalled {
				t.Error("Next handler was not called")
			}

			// Check the response status code
			if rr.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
			}
			if tt.shouldRecord {
				// Check if the duration is greater than the wait time
				if histogram.value < tt.wait.Seconds() {
					t.Errorf("Expected duration to be at least %f seconds, got %f seconds", tt.wait.Seconds(), histogram.value)
				}
			} else {
				// If the path is ignored, we expect no recording
				if histogram.value != 0 {
					t.Errorf("Expected no duration recorded for ignored path, got %f seconds", histogram.value)
				}
			}
		})
	}
}

func TestResponseCounterMetrics(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		method       string
		expectedCode int
		shouldRecord bool
	}{
		{
			name:         "Standard path",
			path:         "/v2/clusters",
			method:       "GET",
			expectedCode: http.StatusOK,
			shouldRecord: true,
		},
		{
			name:         "Ignored path",
			path:         "/v2/healthz",
			method:       "GET",
			expectedCode: http.StatusOK,
			shouldRecord: false, // Ignored path should not increment the counter
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Create a handler with a status code to test counting
			handlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(tt.expectedCode)
			})

			// Create a counter vector for testing
			counter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "test_counter",
				Help: "Test counter for response metrics",
			}, []string{"method", "path", "status"})

			// Apply the middleware
			middlewareFunc := ResponseCounterMetrics(counter, nextHandler)

			// Serve the request
			middlewareFunc.ServeHTTP(rr, req)

			// Verify the handler was called
			if !handlerCalled {
				t.Error("Next handler was not called")
			}

			// Check the response status code
			if rr.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, rr.Code)
			}

			// Check if the counter was incremented correctly (testutil will panic if the metric is not found)
			defer func() {
				if r := recover(); r == nil && !tt.shouldRecord {
					t.Error("Expected panic for ignored path, but did not panic")
				}
			}()
			count := testutil.ToFloat64(counter)
			if tt.shouldRecord {
				if count != 1 {
					t.Error("Expected counter to be incremented, but it was not")
				}
			}
		})
	}
}
