// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/metrics"

	"github.com/stretchr/testify/assert"
)

func TestStatusResponseWriter(t *testing.T) {
	cases := []struct {
		name           string
		writeHeader    int
		writeBody      string
		expectedStatus string
		expectedBody   string
	}{
		{
			name:           "WriteHeader and Write",
			writeHeader:    http.StatusOK,
			writeBody:      "Body and header",
			expectedStatus: "200",
			expectedBody:   "Body and header",
		},
		{
			name:           "WriteHeader only",
			writeHeader:    http.StatusNotFound,
			expectedStatus: "404",
		},
		{
			name:           "Write only",
			writeHeader:    0, // No header written
			writeBody:      "Body only",
			expectedStatus: "0", // Default status
			expectedBody:   "Body only",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			srw := metrics.NewStatusResponseWriter(recorder)

			if tc.writeHeader != 0 {
				srw.WriteHeader(tc.writeHeader)
			}

			if tc.writeBody != "" {
				_, err := srw.Write([]byte(tc.writeBody))
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedStatus, srw.Status())
			assert.Equal(t, tc.expectedBody, recorder.Body.String())
		})
	}
}
