// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/internal/convert"
	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/internal/rest"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

var clusterStatusReady = capi.ClusterStatus{
	Phase: string(capi.ClusterPhaseProvisioned),
	Conditions: []capi.Condition{
		{
			Type:   capi.ReadyCondition,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   capi.ControlPlaneReadyCondition,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   capi.InfrastructureReadyCondition,
			Status: corev1.ConditionTrue,
		},
	},
}

var clusterStatusInProgressControlPlane = capi.ClusterStatus{
	Phase: string(capi.ClusterPhaseProvisioned),
	Conditions: []capi.Condition{
		{
			Type:   capi.ReadyCondition,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   capi.ControlPlaneReadyCondition,
			Status: corev1.ConditionFalse,
		},
		{
			Type:   capi.InfrastructureReadyCondition,
			Status: corev1.ConditionTrue,
		},
	},
}

var clusterStatusErrorControlPlane = capi.ClusterStatus{
	Phase: string(capi.ClusterPhaseProvisioned),
	Conditions: []capi.Condition{
		{
			Type:   capi.ReadyCondition,
			Status: corev1.ConditionTrue,
		},
		{
			Type: capi.ControlPlaneReadyCondition,
		},
		{
			Type:   capi.InfrastructureReadyCondition,
			Status: corev1.ConditionTrue,
		},
	},
}

var clusterStatusUnknown = capi.ClusterStatus{
	Phase:      string(capi.ClusterPhaseProvisioned),
	Conditions: []capi.Condition{},
}

func createMockServer(t *testing.T, clusters []capi.Cluster, projectID string, options ...bool) *rest.Server {
	unstructuredClusters := make([]unstructured.Unstructured, len(clusters))
	for i, cluster := range clusters {
		unstructuredCluster, err := convert.ToUnstructured(cluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")
		unstructuredClusters[i] = *unstructuredCluster
	}
	unstructuredClusterList := &unstructured.UnstructuredList{
		Items: unstructuredClusters,
	}
	// default is to set up k8s client and machineResource mocks
	setupK8sMocks := true
	mockMachineResource := true
	if len(options) > 0 {
		setupK8sMocks = options[0]
		mockMachineResource = options[1]
	}
	var mockedk8sclient *k8s.MockInterface
	mockedk8sclient = k8s.NewMockInterface(t)
	if setupK8sMocks {
		machine := capi.Machine{
			Status: capi.MachineStatus{
				Phase: "Running",
				Conditions: []capi.Condition{
					{Type: "HealthCheckSucceed", Status: "True"},
					{Type: "InfrastructureReady", Status: "True"},
					{Type: "NodeHealthy", Status: "True"},
				},
			},
		}
		unstructuredMachine, err := convert.ToUnstructured(machine)
		require.NoError(t, err, "convertMachineToUnstructured() error = %v, want nil")
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(unstructuredClusterList, nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(projectID).Return(resource)
		mockedk8sclient = k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		if mockMachineResource {
			for _, cluster := range clusters {
				resource.EXPECT().List(mock.Anything, metav1.ListOptions{
					LabelSelector: "cluster.x-k8s.io/cluster-name=" + cluster.Name,
				}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*unstructuredMachine}}, nil).Maybe()
			}
			mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsResource).Maybe()
		}
	}
	return rest.NewServer(mockedk8sclient)
}

func generateClusterWithStatus(name, version *string, status capi.ClusterStatus) capi.Cluster {
	clusterName := ""
	if name != nil {
		clusterName = *name
	}
	clusterVersion := ""
	if version != nil {
		clusterVersion = *version
	}
	return capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec:       capi.ClusterSpec{Topology: &capi.Topology{Version: clusterVersion}},
		Status:     status,
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestGetV2ClustersSummary200(t *testing.T) {
	expectedActiveProjectID := "c61b80c6-f45a-11ef-95cb-5f5567b72329"

	testCases := []struct {
		name             string
		clusters         []capi.Cluster
		expectedResponse api.GetV2ClustersSummary200JSONResponse
	}{
		{
			name:             "No Clusters",
			clusters:         []capi.Cluster{},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 0, Error: 0, InProgress: 0, Unknown: 0, TotalClusters: 0},
		},
		{
			name: "1 ready cluster",
			clusters: []capi.Cluster{
				generateClusterWithStatus(ptr("some name"), ptr("v1.30.6"), clusterStatusReady),
			},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 1, Error: 0, InProgress: 0, Unknown: 0, TotalClusters: 1},
		},
		{
			name: "1 in progress cluster",
			clusters: []capi.Cluster{
				generateClusterWithStatus(ptr("some name"), ptr("v1.30.6"), clusterStatusInProgressControlPlane),
			},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 0, Error: 0, InProgress: 1, Unknown: 0, TotalClusters: 1},
		},
		{
			name: "1 error cluster",
			clusters: []capi.Cluster{
				generateClusterWithStatus(ptr("some name"), ptr("v1.30.6"), clusterStatusErrorControlPlane),
			},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 0, Error: 1, InProgress: 0, Unknown: 0, TotalClusters: 1},
		},
		{
			name: "1 unknown cluster",
			clusters: []capi.Cluster{
				generateClusterWithStatus(ptr("some name"), ptr("v1.30.6"), clusterStatusUnknown),
			},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 0, Error: 0, InProgress: 0, Unknown: 1, TotalClusters: 1},
		},
		{
			name: "every type of cluster",
			clusters: []capi.Cluster{
				generateClusterWithStatus(ptr("some name"), ptr("v1.30.6"), clusterStatusReady),
				generateClusterWithStatus(ptr("in progress cluster"), ptr("v1.30.6"), clusterStatusInProgressControlPlane),
				generateClusterWithStatus(ptr("error cluster"), ptr("v1.30.6"), clusterStatusErrorControlPlane),
				generateClusterWithStatus(ptr("unknown cluster"), ptr("v1.30.6"), clusterStatusUnknown),
			},
			expectedResponse: api.GetV2ClustersSummary200JSONResponse{Ready: 1, Error: 1, InProgress: 1, Unknown: 1, TotalClusters: 4},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := createMockServer(t, tc.clusters, expectedActiveProjectID, true, true)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			req := httptest.NewRequest("GET", "/v2/clusters/summary", nil)
			req.Header.Set("Activeprojectid", expectedActiveProjectID)
			rr := httptest.NewRecorder()

			handler, err := server.ConfigureHandler()
			require.Nil(t, err)
			handler.ServeHTTP(rr, req)

			var actualResponse api.GetV2ClustersSummary200JSONResponse
			err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
			require.NoError(t, err, "Failed to unmarshal response body")

			require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
			require.Equal(t, tc.expectedResponse, actualResponse, "GetV2ClustersSummary() response = %v, want %v", actualResponse, tc.expectedResponse)
		})
	}
}

func TestGetV2ClustersSummary500(t *testing.T) {
	expectedActiveProjectID := "c61b80c6-f45a-11ef-95cb-5f5567b72329"
	t.Run("Failed to Retrieve Clusters", func(t *testing.T) {
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(nil, errors.New("failed to list clusters"))
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		server := rest.NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		req := httptest.NewRequest("GET", "/v2/clusters/summary", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		var actualResponse api.GetV2ClustersSummary500JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 500)
		require.Equal(t, "failed to list clusters", *actualResponse.Message, "Error message = %v, want %v", *actualResponse.Message, "failed to list clusters")
	})
}
