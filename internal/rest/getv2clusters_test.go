// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
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
	Phase: string(capi.ClusterPhaseProvisioning),
	Conditions: []capi.Condition{
		{
			Type:   capi.ReadyCondition,
			Status: corev1.ConditionFalse,
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

var clusterStatusFailed = capi.ClusterStatus{
	Phase: string(capi.ClusterPhaseProvisioning),
	Conditions: []capi.Condition{
		{
			Type:   capi.ConditionType(capi.ClusterPhaseFailed),
			Status: corev1.ConditionTrue,
		},
	},
}

func createMockServer(t *testing.T, clusters []capi.Cluster, projectID string, options ...bool) *Server {
	unstructuredClusters := make([]unstructured.Unstructured, len(clusters))
	machinesList := make([]unstructured.Unstructured, len(clusters))
	for i, cluster := range clusters {
		unstructuredCluster, err := convert.ToUnstructured(cluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")
		unstructuredClusters[i] = *unstructuredCluster
		machine := capi.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: cluster.Name},
			Spec:       capi.MachineSpec{ClusterName: cluster.Name},
			Status:     capi.MachineStatus{Phase: string(capi.MachinePhaseRunning)},
		}
		unstructuredMachine, err := convert.ToUnstructured(machine)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")
		machinesList[i] = *unstructuredMachine
	}
	unstructuredClusterList := &unstructured.UnstructuredList{
		Items: unstructuredClusters,
	}
	unstructuredMachineList := &unstructured.UnstructuredList{
		Items: machinesList,
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
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(unstructuredClusterList, nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(projectID).Return(resource)
		mockedk8sclient = k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)
		if mockMachineResource {
			machineResource := k8s.NewMockResourceInterface(t)
			machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(unstructuredMachineList, nil)
			namespacedMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
			for _, cluster := range clusters {
				machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{
					LabelSelector: "cluster.x-k8s.io/cluster-name=" + cluster.Name,
				}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil).Maybe()
			}
			namespacedMachineResource.EXPECT().Namespace(projectID).Return(machineResource)
			mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(namespacedMachineResource).Maybe()
		}
	}
	return NewServer(mockedk8sclient)
}

func generateCluster(name *string, version *string) capi.Cluster {
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
		Status:     capi.ClusterStatus{Phase: string(capi.ClusterPhaseUnknown)},
	}
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

func generateClusterInfo(name, version string, lifecycleIndicator api.StatusIndicator, lifecycleMessage string) api.ClusterInfo {
	return api.ClusterInfo{
		Name:                ptr(name),
		KubernetesVersion:   ptr(version),
		Labels:              &map[string]interface{}{},
		LifecyclePhase:      &api.GenericStatus{Indicator: statusIndicatorPtr(lifecycleIndicator), Message: ptr(lifecycleMessage), Timestamp: ptr(uint64(0))},
		ControlPlaneReady:   &api.GenericStatus{Indicator: statusIndicatorPtr(lifecycleIndicator), Message: ptr("condition not found"), Timestamp: ptr(uint64(0))},
		InfrastructureReady: &api.GenericStatus{Indicator: statusIndicatorPtr(lifecycleIndicator), Message: ptr("condition not found"), Timestamp: ptr(uint64(0))},
		NodeHealth:          &api.GenericStatus{Indicator: statusIndicatorPtr(api.STATUSINDICATIONIDLE), Message: ptr("nodes are healthy"), Timestamp: ptr(uint64(0))},
		NodeQuantity:        ptr(0),
		ProviderStatus:      &api.GenericStatus{Indicator: statusIndicatorPtr(api.STATUSINDICATIONUNSPECIFIED), Message: ptr("condition not found"), Timestamp: ptr(uint64(0))},
	}
}

func uint64Ptr(num uint64) *uint64 {
	return &num
}

func statusIndicatorPtr(indicator api.StatusIndicator) *api.StatusIndicator {
	return &indicator
}

var expectedActiveProjectID = "655a6892-4280-4c37-97b1-31161ac0b99e"

func TestGetV2Clusters200(t *testing.T) {
	t.Run("No Clusters", func(t *testing.T) {
		clusters := []capi.Cluster{}
		server := createMockServer(t, clusters, expectedActiveProjectID, true, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualClusterDetailInfo api.ClusterDetailInfo
		err = json.Unmarshal(rr.Body.Bytes(), &actualClusterDetailInfo)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

	})
	t.Run("Single Cluster", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster"), ptr("v1.30.6+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID, true, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found")},
			TotalElements: 1,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})

	t.Run("Two Clusters", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})

	t.Run("Three Clusters with One Without Name", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("example-cluster-2"), nil),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("filtered Clusters", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters?filter=name=example-cluster-1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 1,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("ordered clusters - asc (default)", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters?orderBy=name", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("ordered clusters by desc", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters?orderBy=name%20desc", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})

	t.Run("paginated clusters", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("example-cluster-3"), ptr("v1.18.0")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters?pageSize=2&offset=1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		// Parse the response body
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		// check the response content
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-3", "v1.18.0", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 3,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("filtered clusters by name OR kubernetes version", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("example-cluster-3"), ptr("v1.18.0")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with filter by name or kubernetes version
		req := httptest.NewRequest("GET", "/v2/clusters?filter=name=example-cluster-1%20OR%20kubernetesVersion=v1.18.0", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-3", "v1.18.0", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("filtered clusters by name OR kubernetes version", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("cluster-example"), ptr("v1.18.0")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with filter by name prefix or kubernetes version
		req := httptest.NewRequest("GET", "/v2/clusters?filter=name=ster-exam%20OR%20kubernetesVersion=1.2", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("cluster-example", "v1.18.0", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("filtered clusters by name AND specific kubernetes version", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
			generateCluster(ptr("example-cluster-3"), ptr("v1.18.0")),
			generateCluster(ptr("example-cluster-4"), ptr("v1.20.4+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with filter by name prefix and kubernetes version
		// note: the space in the query string is %20 and + is %2B
		req := httptest.NewRequest("GET", "/v2/clusters?filter=name=example-cluster%20AND%20kubernetesVersion=v1.20.4%2Brke2r1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		expectedResponse := api.GetV2Clusters200JSONResponse{
			Clusters: &[]api.ClusterInfo{
				generateClusterInfo("example-cluster-2", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
				generateClusterInfo("example-cluster-4", "v1.20.4+rke2r1", api.STATUSINDICATIONUNSPECIFIED, "Condition not found"),
			},
			TotalElements: 2,
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})

	t.Run("filtered clusters by conditions", func(t *testing.T) {
		tests := []struct {
			name           string
			clusters       []capi.Cluster
			filter         string
			expectedResult api.GetV2Clusters200JSONResponse
		}{
			{
				name: "filtered clusters by providerStatus",
				clusters: []capi.Cluster{
					generateClusterWithStatus(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1"), clusterStatusReady),
					generateClusterWithStatus(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1"), clusterStatusFailed),
				},
				filter: "providerStatus=ready",
				expectedResult: api.GetV2Clusters200JSONResponse{
					Clusters: &[]api.ClusterInfo{
						generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONIDLE, "active"),
					},
					TotalElements: 1,
				},
			},
			{
				name: "filtered clusters by lifecyclePhase",
				clusters: []capi.Cluster{
					generateClusterWithStatus(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1"), clusterStatusReady),
					generateClusterWithStatus(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1"), clusterStatusInProgressControlPlane),
				},
				filter: "lifecyclePhase=active",
				expectedResult: api.GetV2Clusters200JSONResponse{
					Clusters: &[]api.ClusterInfo{
						generateClusterInfo("example-cluster-1", "v1.30.6+rke2r1", api.STATUSINDICATIONIDLE, "active"),
					},
					TotalElements: 1,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := createMockServer(t, tt.clusters, expectedActiveProjectID)
				require.NotNil(t, server, "NewServer() returned nil, want not nil")

				// Create a new request & response recorder
				req := httptest.NewRequest("GET", "/v2/clusters?filter="+tt.filter, nil)
				req.Header.Set("Activeprojectid", expectedActiveProjectID)
				rr := httptest.NewRecorder()

				// create a handler with middleware to serve the request
				handler, err := server.ConfigureHandler()
				require.Nil(t, err)
				handler.ServeHTTP(rr, req)

				// Parse the response body
				var actualResponse api.GetV2Clusters200JSONResponse
				err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
				assert.NoError(t, err, "Failed to unmarshal response body")

				// Check the response status
				assert.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

				for _, cluster := range *actualResponse.Clusters {
					cluster.ControlPlaneReady.Message = ptr("condition not found")
					cluster.ControlPlaneReady.Timestamp = ptr(uint64(0))

					cluster.InfrastructureReady.Message = ptr("condition not found")
					cluster.InfrastructureReady.Timestamp = ptr(uint64(0))

					cluster.ProviderStatus.Indicator = statusIndicatorPtr(api.STATUSINDICATIONUNSPECIFIED)
					cluster.ProviderStatus.Message = ptr("condition not found")
					cluster.ProviderStatus.Timestamp = ptr(uint64(0))

					cluster.NodeHealth.Timestamp = ptr(uint64(0))
					cluster.LifecyclePhase.Timestamp = ptr(uint64(0))
				}

				// Check the response content
				assert.Equal(t, tt.expectedResult, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, tt.expectedResult)
			})
		}
	})

	t.Run("no clusters after filter criteria", func(t *testing.T) {
		clusters := []capi.Cluster{
			generateCluster(ptr("example-cluster-1"), ptr("v1.30.6+rke2r1")),
			generateCluster(ptr("example-cluster-2"), ptr("v1.20.4+rke2r1")),
		}
		server := createMockServer(t, clusters, expectedActiveProjectID)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with a filter that matches no clusters
		req := httptest.NewRequest("GET", "/v2/clusters?filter=name=nonexistent-cluster", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualClusterDetailInfo api.ClusterDetailInfo
		err = json.Unmarshal(rr.Body.Bytes(), &actualClusterDetailInfo)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
	})
}

func TestGetV2Clusters500(t *testing.T) {
	t.Run("Failed to Retrieve Clusters", func(t *testing.T) {
		// Simulate an error in listing clusters
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(nil, errors.New("failed to list clusters"))
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters500JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 500)

		// Check the error message
		require.Equal(t, "failed to retrieve clusters", *actualResponse.Message, "Error message = %v, want %v",
			*actualResponse.Message, "failed to retrieve clusters")
	})

	t.Run("Missing Project ID", func(t *testing.T) {
		// Simulate a missing project ID in the context
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(nil, errors.New("failed to list clusters")).Maybe()
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource).Maybe()

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var actualResponse api.GetV2Clusters500JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 500)

		// Check the error message
		require.Equal(t, "failed to retrieve clusters", *actualResponse.Message, "Error message = %v, want %v",
			*actualResponse.Message, "failed to retrieve clusters")
	})
}

func TestGetV2Clusters400(t *testing.T) {
	t.Run("Nil Context", func(t *testing.T) {
		// Simulate an error in listing clusters
		mockedk8sclient := k8s.NewMockInterface(t)

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		// Do not set the Activeprojectid header to simulate the missing project ID
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response status
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 400)

		// Check the error message in the response body
		var respbody api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &respbody)
		require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
		expectedErrorMessage := "no active project id provided"
		require.Contains(t, *respbody.Message, expectedErrorMessage, "Error message = %v, want %v",
			*respbody.Message, expectedErrorMessage)
	})

	t.Run("No Project ID", func(t *testing.T) {
		// Simulate an error in listing clusters
		mockedk8sclient := k8s.NewMockInterface(t)

		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters", nil)
		// Set an empty Activeprojectid header to simulate no project ID
		req.Header.Set("Activeprojectid", "")
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Print the response body for debugging
		t.Logf("Response body: %s", rr.Body.String())

		// Check the response status
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 400)

		// Check the error message in the response body
		var respbody api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &respbody)
		require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
		expectedErrorMessage := "no active project id provided"
		require.Contains(t, *respbody.Message, expectedErrorMessage, "Error message = %v, want %v",
			*respbody.Message, expectedErrorMessage)
	})
	t.Run("invalid pageSize", func(t *testing.T) {
		server := createMockServer(t, []capi.Cluster{}, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with invalid pageSize
		// this error is being capture at http handler with strict validation against the OpenAPI spec
		req := httptest.NewRequest("GET", "/v2/clusters?pageSize=-1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("parameter \"pageSize\" in query has an error: number must be at least 0"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("invalid combination of pageSize and offset", func(t *testing.T) {
		clusters := []capi.Cluster{}
		server := createMockServer(t, clusters, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// create a new request & response recorder with invalid combination of pageSize and offset
		// this is not blocked in the OpenAPI spec strict validation, but by the validateParams
		req := httptest.NewRequest("GET", "/v2/clusters?pageSize=0&offset=1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("invalid pageSize: must be greater than 0"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("invalid orderBy field", func(t *testing.T) {
		clusters := []capi.Cluster{}
		server := createMockServer(t, clusters, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with invalid orderBy field
		// this error is being capture at http handler with strict validation against the OpenAPI spec
		req := httptest.NewRequest("GET", "/v2/clusters?orderBy=", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("parameter \"orderBy\" in query has an error: empty value is not allowed"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})

	t.Run("invalid orderBy field", func(t *testing.T) {
		clusters := []capi.Cluster{}
		server := createMockServer(t, clusters, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with syntactically correct but logically invalid orderBy field
		req := httptest.NewRequest("GET", "/v2/clusters?orderBy=nonexistentField%20desc", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("invalid orderBy field"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("Invalid OrderBy Parameter", func(t *testing.T) {
		server := createMockServer(t, []capi.Cluster{}, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with an invalid orderBy parameter
		req := httptest.NewRequest("GET", "/v2/clusters?orderBy=name%20badasc", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 400)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("invalid orderBy field"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
	t.Run("invalid filter", func(t *testing.T) {
		clusters := []capi.Cluster{}
		server := createMockServer(t, clusters, expectedActiveProjectID, false, true)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder with invalid filter
		req := httptest.NewRequest("GET", "/v2/clusters?filter=invalidFilter", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		var actualResponse api.GetV2Clusters400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &actualResponse)
		require.NoError(t, err, "Failed to unmarshal response body")
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
		expectedResponse := api.GetV2Clusters400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("invalid filter field"),
			},
		}
		require.Equal(t, expectedResponse, actualResponse, "GetV2Clusters() response = %v, want %v", actualResponse, expectedResponse)
	})
}

func createGetV2ClustersStubServer(t *testing.T) *Server {
	unstructuredClusters := make([]unstructured.Unstructured, 0)
	unstructuredClusterList := &unstructured.UnstructuredList{
		Items: unstructuredClusters,
	}
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(unstructuredClusterList, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()

	// Add mock for machines resource to support fetchAllMachinesList
	unstructuredMachines := make([]unstructured.Unstructured, 0)
	unstructuredMachineList := &unstructured.UnstructuredList{
		Items: unstructuredMachines,
	}
	machineResource := k8s.NewMockResourceInterface(t)
	machineResource.EXPECT().List(mock.Anything, mock.Anything).Return(unstructuredMachineList, nil).Maybe()
	nsMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsMachineResource.EXPECT().Namespace(mock.Anything).Return(machineResource).Maybe()

	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource).Maybe()
	mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsMachineResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzGetV2Clusters(f *testing.F) {
	f.Add(1, 2, "abc", "def",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, pageSize, offset int, orderBy, filter string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2ClustersStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.GetV2ClustersRequestObject{
			Params: api.GetV2ClustersParams{
				PageSize:        &pageSize,
				Offset:          &offset,
				OrderBy:         &orderBy,
				Filter:          &filter,
				Activeprojectid: activeprojectid,
			},
		}
		server.GetV2Clusters(context.Background(), req)
	})
}
