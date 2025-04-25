// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func setupMockServer1(t *testing.T, clusterName string, expectedActiveProjectID string, existingLabels map[string]string, getError error, updateError error) *Server {
    // Create the mock client
    mockK8sClient := k8s.NewMockClient(t)

    // If no getError, mock the Cluster method (not GetCluster)
    if getError == nil {
        // If no update error, also mock the CreateClusterLabels method (not UpdateClusterLabels)
        if updateError == nil {
            mockK8sClient.EXPECT().CreateClusterLabels(mock.Anything, expectedActiveProjectID, clusterName, mock.Anything).Return(nil)
        } else {
            mockK8sClient.EXPECT().CreateClusterLabels(mock.Anything,expectedActiveProjectID, clusterName, mock.Anything).Return(updateError)
        }
    } else {
		mockK8sClient.EXPECT().CreateClusterLabels(mock.Anything, expectedActiveProjectID, clusterName, mock.Anything).Return(getError)
    }

    // Create and return the server
    server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
    require.NotNil(t, server, "NewServer() returned nil, want not nil")

    return server
}

func TestPutV2ClusterLabels200(t *testing.T) {
    // Define test data
    expectedClusterName := "example-cluster"
    existingLabels := map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"}
    expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

    // Create mock server with test data
    server := setupMockServer1(t, expectedClusterName, expectedActiveProjectID, existingLabels, nil, nil)

    // Create a new request & response recorder
    reqBody := api.ClusterLabels{
        Labels: &map[string]string{"key1": "value1"},
    }
    reqBodyBytes, err := json.Marshal(reqBody)
    require.NoError(t, err, "Failed to marshal request body")

    req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Activeprojectid", expectedActiveProjectID)
    rr := httptest.NewRecorder()

    // Create a handler with middleware to serve the request
    handler, err := server.ConfigureHandler()
    require.Nil(t, err)
    handler.ServeHTTP(rr, req)

    // Check the response status
    require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

    // Check if the response body is empty
    require.Empty(t, rr.Body.Bytes(), "Expected empty response body")
}

func TestPutV2ClusterLabels400(t *testing.T) {
    t.Run("ValidationFailure", func(t *testing.T) {
        // Create a server with a nil client since validation happens before client calls
        server := NewServer(nil, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
        
        expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

        // Create a new request with invalid label keys
        reqBody := api.ClusterLabels{
            Labels: &map[string]string{"invalid key!": "value1"},
        }
        reqBodyBytes, err := json.Marshal(reqBody)
        require.NoError(t, err, "failed to marshal request body")

        req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Activeprojectid", expectedActiveProjectID)
        rr := httptest.NewRecorder()

        // Create a handler with middleware to serve the request
        handler, err := server.ConfigureHandler()
        require.Nil(t, err)
        handler.ServeHTTP(rr, req)

        // Check the response status
        require.Equal(t, http.StatusBadRequest, rr.Code)

        // Validate the error message 
        expectedErrorMessage := `{"message":"invalid cluster label keys"}`
        require.JSONEq(t, expectedErrorMessage, rr.Body.String())
    })

    t.Run("InvalidCluster", func(t *testing.T) {
        expectedCluster := unstructured.Unstructured{}
        expectedCluster.SetName("example-cluster")
        expectedCluster.SetLabels(map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"})
        expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

        // Simulate a bad request error in the Update method
        server := setupMockServer1(t, expectedCluster.GetName(), expectedActiveProjectID, expectedCluster.GetLabels(), nil, k8serrors.NewBadRequest("invalid cluster"))

        // Create a new request & response recorder
        reqBody := api.ClusterLabels{
            Labels: &map[string]string{"key1": "value1"},
        }
        reqBodyBytes, err := json.Marshal(reqBody)
        require.NoError(t, err, "failed to marshal request body")

        req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Activeprojectid", expectedActiveProjectID)
        rr := httptest.NewRecorder()

        // Create a handler with middleware to serve the request
        handler, err := server.ConfigureHandler()
        require.Nil(t, err)
        handler.ServeHTTP(rr, req)

        // Check the response status
        require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)

        // Validate the error message in the response body
        expectedErrorMessage := `{"message":"cluster 'example-cluster' is invalid: invalid cluster"}`
        require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
    })

    t.Run("InvalidActiveProjectID", func(t *testing.T) {
        expectedActiveProjectID := "00000000-0000-0000-0000-000000000000"
        testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

        server := NewServer(nil) // Pass nil or a mock client if needed
        require.NotNil(t, server, "NewServer() returned nil, want not nil")

        // Create a handler with middleware
        handler, err := server.ConfigureHandler()
        require.Nil(t, err)

        // Create a new request & response recorder
        req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/clusters/invalid-name/labels", strings.NewReader(`{"labels": {"key": "value"}}`))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Activeprojectid", expectedActiveProjectID)

       	rr := httptest.NewRecorder()

        // Serve the request
        handler.ServeHTTP(rr, req)
        require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)

        // Validate the error message in the response body
        expectedErrorMessage := `{"message": "no active project id provided"}`
        require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
    })
}

func TestPutV2ClusterLabel404(t *testing.T) {
    // Define test data
    expectedClusterName := "example-cluster"
    existingLabels := map[string]string{"key1": "value1"}
    expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
    
    // Create not found error
    notFoundError := k8serrors.NewNotFound(schema.GroupResource{Group: "clusters", Resource: "example-cluster"}, "example-cluster")

    // Create mock server that returns not found error
    server := setupMockServer1(t, expectedClusterName, expectedActiveProjectID, existingLabels, notFoundError, nil)

    // Create a new request & response recorder
    reqBody := api.ClusterLabels{
        Labels: &map[string]string{"key1": "value1"},
    }
    reqBodyBytes, err := json.Marshal(reqBody)
    require.NoError(t, err, "failed to marshal request body")

    req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Activeprojectid", expectedActiveProjectID)
    rr := httptest.NewRecorder()

    // Create a handler with middleware to serve the request
    handler, err := server.ConfigureHandler()
    require.Nil(t, err)
    handler.ServeHTTP(rr, req)
    
    // Check the response status
    require.Equal(t, http.StatusNotFound, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusNotFound)

    // Validate the error message in the response body
    expectedErrorMessage := `{"message":"cluster 'example-cluster' not found: example-cluster.clusters \"example-cluster\" not found"}`
    require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
}

func TestPutV2ClusterLabels500(t *testing.T) {
    // Define test data
    expectedClusterName := "example-cluster"
    existingLabels := map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"}
    expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
    
    // Create internal server error
    internalError := errors.New("internal server error")

    // Create mock server that returns internal error on update
    server := setupMockServer1(t, expectedClusterName, expectedActiveProjectID, existingLabels, nil, internalError)

    // Create a new request & response recorder
    reqBody := api.ClusterLabels{
        Labels: &map[string]string{"key1": "value1"},
    }
    reqBodyBytes, err := json.Marshal(reqBody)
    require.NoError(t, err, "failed to marshal request body")

    req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Activeprojectid", expectedActiveProjectID)
    rr := httptest.NewRecorder()

    // Create a handler with middleware to serve the request
    handler, err := server.ConfigureHandler()
    require.Nil(t, err)
    handler.ServeHTTP(rr, req)

    // Check the response status
    require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

    // Validate the error message in the response body
    expectedErrorMessage := `{"message":"failed to update Cluster 'example-cluster': internal server error"}`
    require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
}
