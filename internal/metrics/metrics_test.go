// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	cases := []struct {
		name           string
		setup          func()
		expectedStatus int
		expectedBody   []string
	}{
		{
			name: "TestResponseTimeMetric1",
			setup: func() {
				metrics.ResponseTime.Observe(1.23)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []string{
				"cluster_manager_http_response_time_seconds_histogram",
				"1.23",
			},
		},
		{
			name: "TestResponseTimeMetric2",
			setup: func() {
				metrics.ResponseTime.Observe(2.34)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []string{
				"cluster_manager_http_response_time_seconds_histogram",
				"3.57",
			},
		},
		{
			name: "TestHttpResponseCounterMetric1",
			setup: func() {
				metrics.HttpResponseCounter.WithLabelValues("GET", "/test1", "200").Inc()
			},
			expectedStatus: http.StatusOK,
			expectedBody: []string{
				"cluster_manager_http_response_codes_counter",
				`code="200",method="GET",path="/test1"`,
			},
		},
		{
			name: "TestHttpResponseCounterMetric2",
			setup: func() {
				metrics.HttpResponseCounter.WithLabelValues("GET", "/test2", "404").Inc()
			},
			expectedStatus: http.StatusOK,
			expectedBody: []string{
				"cluster_manager_http_response_codes_counter",
				`code="200",method="GET",path="/test1"`,
				`code="404",method="GET",path="/test2"`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := metrics.GetRegistry()

			tc.setup()

			handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.expectedStatus, rec.Code)

			body := rec.Body.String()
			for _, expected := range tc.expectedBody {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestGetRegistry(t *testing.T) {
	registry := metrics.GetRegistry()
	assert.NotNil(t, registry, "Registry should not be nil")

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Error gathering metrics")
	assert.True(t, len(metricFamilies) > 0, "No metrics found in the registry")
}
