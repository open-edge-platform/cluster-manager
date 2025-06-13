// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectIDValidator(t *testing.T) {
	tests := []struct {
		name           string
		projectID      string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid project ID",
			projectID:      "12345678-1234-1234-1234-123456789012",
			path:           "/v2/clusters",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
		{
			name:           "Empty project ID",
			projectID:      "",
			path:           "/v2/clusters",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"message": "no active project id provided"}`,
		},
		{
			name:           "Zero UUID project ID",
			projectID:      "00000000-0000-0000-0000-000000000000",
			path:           "/v2/clusters",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"message": "no active project id provided"}`,
		},
		{
			name:           "Ignored /healthz path",
			projectID:      "",
			path:           "/v2/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
		{
			name:           "Ignored /healthz path with project ID",
			projectID:      "12345678-1234-1234-1234-123456789012",
			path:           "/v2/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
		{
			name:           "Ignored metrics path with project ID",
			projectID:      "12345678-1234-1234-1234-123456789012",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
		{
			name:           "Ignored metrics path without project ID",
			projectID:      "",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			expectedBody:   "handler called",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the specified path and header
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Set project ID header if provided
			if tt.projectID != "" {
				req.Header.Set("Activeprojectid", tt.projectID)
			}

			// Create a recorder to capture the response
			rr := httptest.NewRecorder()

			// Create a handler that the middleware will call if validation passes
			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.Write([]byte("handler called"))
			})

			// Apply the middleware to the handler
			middlewareFunc := ProjectIDValidator(handler)

			// Serve the request
			middlewareFunc.ServeHTTP(rr, req)

			// Check the status code
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}

			// Check if the handler was called when expected
			if tt.expectedStatus == http.StatusOK && !handlerCalled {
				t.Errorf("handler was not called, but it should have been")
			}

			// For error cases, check the response body
			if tt.expectedStatus != http.StatusOK {
				if rr.Body.String() != tt.expectedBody+"\n" {
					t.Errorf("handler returned unexpected body: got %v want %v",
						rr.Body.String(), tt.expectedBody+"\n")
				}
			}
		})
	}
}
