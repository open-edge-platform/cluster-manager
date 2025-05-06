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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

// (DELETE /v2/clusters/{name}/nodes/{nodeId})
func TestDeleteClustersNameNodeId(t *testing.T) {
	t.Run("Successful Node Deletion on Single Node Cluster", func(t *testing.T) {
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Mock the delete cluster to succeed
		clusterResource := k8s.NewMockResourceInterface(t)
		mockInterface := k8s.NewMockInterface(t)
		clusterResource.EXPECT().Delete(mock.Anything, name, metav1.DeleteOptions{}).Return(nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)

		mockedk8sclient := k8s.NewMockClient(t)
		mockedk8sclient.EXPECT().Dynamic().Return(mockInterface)
		mockInterface.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		mockedk8sclient.EXPECT().GetCluster(mock.Anything, activeProjectID, name).Return(&capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: activeProjectID,
			},
			Spec: capi.ClusterSpec{
				Topology: &capi.Topology{
					Class:   "default-template",
					Version: "v1",
				},
			},
		}, nil)
		mockedk8sclient.EXPECT().ListCached(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&unstructured.UnstructuredList{
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
								"name": "machine1",
							},
						},
					},
				},
			},
		}, nil)
		mockedk8sclient.EXPECT().GetMachines(mock.Anything, activeProjectID, name).Return([]capi.Machine{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: activeProjectID,
					Annotations: map[string]string{
						"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
					},
				},
				Status: capi.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						UID: types.UID(nodeID),
					},
				},

				Spec: capi.MachineSpec{
					ClusterName: name,
					InfrastructureRef: corev1.ObjectReference{
						Kind: "IntelMachine",
						Name: "machine1",
					},
				},
			},
		}, nil)
		mockedk8sclient.EXPECT().IntelMachine(mock.Anything, activeProjectID, "machine1").Return(intelProvider.IntelMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine1",
				Namespace: activeProjectID,
				Annotations: map[string]string{
					"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
				},
			},
		}, nil)
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

		mockK8sClient := k8s.NewMockClient(t)
		mockK8sClient.EXPECT().ListCached(mock.Anything, mock.Anything, activeProjectID, metav1.ListOptions{LabelSelector: "cluster.x-k8s.io/cluster-name=" + name}).Return(&unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"intelmachine.infrastructure.cluster.x-k8s.io/host-id": "different-id",
							},
						},
					},
				},
			},
		}, nil)

		mockK8sClient.EXPECT().GetCluster(mock.Anything, activeProjectID, name).Return(&capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: activeProjectID,
			},
			Spec: capi.ClusterSpec{
				Topology: &capi.Topology{
					Class:   "default-template",
					Version: "v1",
				},
			},
		}, nil)

		mockK8sClient.EXPECT().GetMachines(mock.Anything, activeProjectID, name).Return([]capi.Machine{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: activeProjectID,
					Annotations: map[string]string{
						"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
					},
				},
				Status: capi.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						UID: types.UID(nodeID),
					},
				},

				Spec: capi.MachineSpec{
					ClusterName: name,
					InfrastructureRef: corev1.ObjectReference{
						Kind: "IntelMachine",
						Name: "machine1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine2",
					Namespace: activeProjectID,
					Annotations: map[string]string{
						"intelmachine.infrastructure.cluster.x-k8s.io/host-id": "other-id",
					},
				},
				Status: capi.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						UID: types.UID(nodeID),
					},
				},

				Spec: capi.MachineSpec{
					ClusterName: name,
					InfrastructureRef: corev1.ObjectReference{
						Kind: "IntelMachine",
						Name: "machine2",
					},
				},
			},
		}, nil)

		mockK8sClient.EXPECT().IntelMachine(mock.Anything, activeProjectID, mock.Anything).Return(intelProvider.IntelMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine1",
				Namespace: activeProjectID,
				Annotations: map[string]string{
					"intelmachine.infrastructure.cluster.x-k8s.io/host-id": nodeID,
				},
			},
		}, nil)
		// Create a new server with the mocked k8s client
		server := NewServer(mockK8sClient)
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
	t.Run("Cluster Not Found", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nodeID := "535436e4-4b0b-4b3b-8b3b-3b3b3b3b3b3b"

		// Create mock objects
		mockK8sClient := k8s.NewMockClient(t)

		// Set up mock expectations
		mockK8sClient.EXPECT().GetCluster(mock.Anything, activeProjectID, name).Return(nil, fmt.Errorf("cluster not found"))

		// Create a new server with the mocked k8s client
		server := NewServer(mockK8sClient)
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
}
