// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

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
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

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

		server := NewServer(wrapMockInterface(mockedk8sclient))
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
