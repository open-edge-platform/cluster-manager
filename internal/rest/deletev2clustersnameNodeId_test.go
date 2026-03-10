// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
)

// (DELETE /v2/clusters/{name}/nodes/{nodeId})
func TestDeleteClustersNameNodeId(t *testing.T) {
	t.Run("Successful Node Deletion on Single Node Cluster", func(t *testing.T) {
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Mock the delete cluster to succeed
		clusterResource := k8s.NewMockResourceInterface(t)
		machineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: map[string]interface{}{}}, nil)
		clusterResource.EXPECT().Delete(mock.Anything, name, metav1.DeleteOptions{}).Return(nil)
		clusterResource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Cluster",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": activeProjectID,
				},
				"spec": map[string]interface{}{
					"nodeID": nodeID,
					"topology": map[string]interface{}{
						"version": "v1",
						"class":   "topology.kubernetes.io/zone",
					},
					"providerStatus": map[string]interface{}{
						"indicator": "Ready",
					},
					"lifecyclePhase": map[string]interface{}{
						"indicator": "Active",
					},
					"nodeHealth": map[string]interface{}{
						"indicator": "Healthy",
					},
					"template": "default-template",
				},
				"labels": map[string]interface{}{
					"env": "production",
				},
			},
		}, nil)
		machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(&unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "cluster.k8s.io/v1alpha1",
						"kind":       "Machine",
						"metadata": map[string]interface{}{
							"name":      "machine1",
							"namespace": activeProjectID,
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
							},
						},
						"status": map[string]interface{}{
							"nodeInfo": map[string]interface{}{
								"systemUUID": nodeID,
							},
						},
						"spec": map[string]interface{}{
							"clusterName": name,
							"infrastructureRef": map[string]interface{}{
								"kind": "IntelMachine",
							},
						},
					},
				},
			},
		}, nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedIntelMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedMachine.EXPECT().Namespace(activeProjectID).Return(machineResource)
		nsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)
		namespacedIntelMachine.EXPECT().Namespace(activeProjectID).Return(intelMachineResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(namespacedMachine)
		mockedk8sclient.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(namespacedIntelMachine)

		// Create a new server with the mocked k8s client
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusOK, rr.Code)
	})
	t.Run("Error on Multi Node Cluster", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"
		// Mock the delete cluster to succeed
		clusterResource := k8s.NewMockResourceInterface(t)
		machineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: map[string]interface{}{}}, nil)
		intelMachineResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(&unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "cluster.k8s.io/v1alpha1",
						"kind":       "IntelMachine",
						"metadata": map[string]interface{}{
							"name":      "machine1",
							"namespace": activeProjectID,
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
							},
							"finalizers": []string{"intelmachine.infrastructure.cluster.x-k8s.io/host-cleanup"},
						},
						"status": map[string]interface{}{
							"nodeRef": map[string]interface{}{
								"uid": nodeID,
							},
						},
						"spec": map[string]interface{}{},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "cluster.k8s.io/v1alpha1",
						"kind":       "IntelMachine",
						"metadata": map[string]interface{}{
							"name":      "machine2",
							"namespace": activeProjectID,
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": "different-id",
							},
						},
						"status": map[string]interface{}{
							"nodeRef": map[string]interface{}{
								"uid": "different-node-id",
							},
						},
						"spec": map[string]interface{}{},
					},
				},
			},
		}, nil)
		intelMachineResource.EXPECT().Patch(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&unstructured.Unstructured{}, nil)
		clusterResource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Cluster",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": activeProjectID,
				},
				"spec": map[string]interface{}{
					"nodeID": nodeID,
					"topology": map[string]interface{}{
						"version": "v1",
						"class":   "topology.kubernetes.io/zone",
						"controlPlane": map[string]interface{}{
							"replicas": 2,
						},
					},
					"providerStatus": map[string]interface{}{
						"indicator": "Ready",
					},
					"lifecyclePhase": map[string]interface{}{
						"indicator": "Active",
					},
					"nodeHealth": map[string]interface{}{
						"indicator": "Healthy",
					},
					"template": "default-template",
				},
				"labels": map[string]interface{}{
					"env": "production",
				},
			},
		}, nil)
		machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(&unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "cluster.k8s.io/v1alpha1",
						"kind":       "Machine",
						"metadata": map[string]interface{}{
							"name":      "machine1",
							"namespace": activeProjectID,
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
							},
						},
						"status": map[string]interface{}{
							"nodeRef": map[string]interface{}{
								"uid": nodeID,
							},
						},
						"spec": map[string]interface{}{
							"clusterName": name,
							"infrastructureRef": map[string]interface{}{
								"kind": "IntelMachine",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "cluster.k8s.io/v1alpha1",
						"kind":       "Machine",
						"metadata": map[string]interface{}{
							"name":      "machine2",
							"namespace": activeProjectID,
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": "different-id",
							},
						},
						"status": map[string]interface{}{
							"nodeRef": map[string]interface{}{
								"uid": "different-node-id",
							},
						},
						"spec": map[string]interface{}{
							"clusterName": name,
							"infrastructureRef": map[string]interface{}{
								"kind": "IntelMachine",
							},
						},
					},
				},
			},
		}, nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedIntelMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedMachine.EXPECT().Namespace(activeProjectID).Return(machineResource)
		nsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)
		namespacedIntelMachine.EXPECT().Namespace(activeProjectID).Return(intelMachineResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(namespacedMachine)
		mockedk8sclient.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(namespacedIntelMachine)
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s?force=true", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message": "multi node clusters are not supported"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())

	})
}

func TestDeleteClustersNameNodeId400(t *testing.T) {
	t.Run("Missing Active Project ID", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Create a new server
		server := NewServer(nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s", name, nodeID), nil)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message": "no active project id provided"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

func TestDeleteClustersNameNodeId500(t *testing.T) {
	t.Run("Force Delete - IntelMachines namespace not found returns 404", func(t *testing.T) {
		name := "fuzzstring"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Simulate k8s returning a NotFound error on the IntelMachines list
		notFoundErr := k8serrors.NewNotFound(schema.GroupResource{Group: "infrastructure.cluster.x-k8s.io", Resource: "intelmachines"}, name)
		intelMachineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(nil, notFoundErr)
		namespacedIntelMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedIntelMachine.EXPECT().Namespace(activeProjectID).Return(intelMachineResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(namespacedIntelMachine)

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server)

		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s?force=true", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		expectedResponse := fmt.Sprintf(`{"message": "cluster or project not found: %s/%s"}`, activeProjectID, name)
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
	t.Run("Force Delete - IntelMachines backend error returns 500", func(t *testing.T) {
		name := "fuzzstring"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Simulate a generic backend error (not a NotFound) on the IntelMachines list
		intelMachineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(nil, fmt.Errorf("connection refused"))
		namespacedIntelMachine := k8s.NewMockNamespaceableResourceInterface(t)
		namespacedIntelMachine.EXPECT().Namespace(activeProjectID).Return(intelMachineResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(namespacedIntelMachine)

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server)

		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s?force=true", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message": "failed to retrieve intel machines"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
	t.Run("Cluster Not Found", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Mock the get cluster to fail
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(nil, fmt.Errorf("cluster not found"))
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message": "failed to retrieve cluster"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
	t.Run("Cluster k8s NotFound returns 404", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Mock the get cluster to return the sentinel ErrClusterNotFound (as cli.GetCluster does)
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(nil, k8serrors.NewNotFound(schema.GroupResource{Group: "cluster.x-k8s.io", Resource: "clusters"}, name))
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s/nodes/%s", name, nodeID), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// A k8s NotFound error should map to 404 with a structured message
		assert.Equal(t, http.StatusNotFound, rr.Code)
		expectedResponse := fmt.Sprintf(`{"message": "cluster not found: %s/%s"}`, activeProjectID, name)
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}
