// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/rest"
	"github.com/stretchr/testify/require"
)

func TestPutV2ClustersNameTemplate(t *testing.T) {
	// create a server with nil k8s client
	server := rest.NewServer(nil)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// call the PutV2ClustersNameTemplate method
	body := `
	{
		"name": "baseline",
		"version": "v0.1.0"
	}`
	req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/template", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", "655a6892-4280-4c37-97b1-31161ac0b99e")
	rr := httptest.NewRecorder()
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)

	// check the response
	require.Equal(t, http.StatusNotImplemented, rr.Code)
	require.JSONEq(t, `{"message":"In-place cluster updates are not supported. Please delete the cluster and create a new one with updated cluster template."}`, rr.Body.String())
}
