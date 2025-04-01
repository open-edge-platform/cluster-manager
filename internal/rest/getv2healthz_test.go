// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetV2Healthz(t *testing.T) {
	// Create a server instance
	server := NewServer(nil)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a new request & response recorder
	ctx := context.Background()
	req := httptest.NewRequest("GET", "/v2/healthz", nil)
	rr := httptest.NewRecorder()

	// Create a handler with middleware to serve the request
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req.WithContext(ctx))

	// Check the response
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)
	expectedResponse := "\"cm rest server is healthy\"\n"
	assert.Equal(t, expectedResponse, rr.Body.String())
}
