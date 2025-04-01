// / SPDX-FileCopyrightText: (C) 2025 Intel Corporation
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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

func setupMockServer1(t *testing.T, expectedCluster unstructured.Unstructured, expectedActiveProjectID string, getReturn *unstructured.Unstructured, getError error, updateError error) *Server {
	// configure mockery to return the test cluster object on a Get() call
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, expectedCluster.GetName(), v1.GetOptions{}).Return(getReturn, getError)
	if getError == nil {
		resource.EXPECT().Update(mock.Anything, mock.MatchedBy(func(obj *unstructured.Unstructured) bool {
			return true
		}), v1.UpdateOptions{}).Return(getReturn, updateError)
	}
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

	// create a new server with the mocked mockedk8sclient
	server := NewServer(mockedk8sclient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	return server
}
func TestPutV2ClusterLabels200(t *testing.T) {
	expectedCluster := unstructured.Unstructured{}
	expectedCluster.SetName("example-cluster")
	expectedCluster.SetLabels(map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"})
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

	// Convert the expected cluster to an unstructured object
	unstructuredCluster := &expectedCluster

	server := setupMockServer1(t, expectedCluster, expectedActiveProjectID, unstructuredCluster, nil, nil)

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

	// create a handler with middleware to serve the request
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
		expectedCluster := unstructured.Unstructured{}
		expectedCluster.SetName("example-cluster")
		expectedCluster.SetLabels(map[string]string{"key1": "value1"})
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		// Create a new request & response recorder with invalid label keys
		reqBody := api.ClusterLabels{
			Labels: &map[string]string{"invalid key!": "value1"},
		}
		reqBodyBytes, err := json.Marshal(reqBody)
		require.NoError(t, err, "failed to marshal request body")

		req := httptest.NewRequest("PUT", "/v2/clusters/example-cluster/labels", bytes.NewReader(reqBodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// Create a server instance
		server := NewServer(nil) // Pass nil or a mock client if needed

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response status
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)

		// Validate the error message in the response body
		expectedErrorMessage := `{"message":"invalid cluster label keys"}`
		require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
	})

	t.Run("InvalidCluster", func(t *testing.T) {
		expectedCluster := unstructured.Unstructured{}
		expectedCluster.SetName("example-cluster")
		expectedCluster.SetLabels(map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"})
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		// Simulate a bad request error in the Update method
		server := setupMockServer1(t, expectedCluster, expectedActiveProjectID, &expectedCluster, nil, k8serrors.NewBadRequest("invalid cluster"))

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
	expectedCluster := unstructured.Unstructured{}
	expectedCluster.SetName("example-cluster")
	expectedCluster.SetLabels(map[string]string{"key1": "value1"})
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

	// Simulate a not found error in the Get method
	server := setupMockServer1(t, expectedCluster, expectedActiveProjectID, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "clusters", Resource: "example-cluster"}, "example-cluster"), nil)

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

	// create a handler with middleware to serve the request
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
	expectedCluster := unstructured.Unstructured{}
	expectedCluster.SetName("example-cluster")
	expectedCluster.SetLabels(map[string]string{"user.edge-orchestrator.intel.com/key1": "value1"})
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

	// Convert the expected cluster to an unstructured object
	unstructuredCluster := &expectedCluster

	// Simulate an internal server error in the Update method
	server := setupMockServer1(t, expectedCluster, expectedActiveProjectID, unstructuredCluster, nil, errors.New("internal server error"))

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

	// create a handler with middleware to serve the request
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)

	// Check the response status
	require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

	// Validate the error message in the response body
	expectedErrorMessage := `{"message":"failed to update Cluster 'example-cluster': internal server error"}`
	require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
}
