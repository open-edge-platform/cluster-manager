// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

var activeProjectID = "655a6892-4280-4c37-97b1-31161ac0b99e"
var hostId = "host-4228d4f1c2bd"
var clusterLabels = map[string]string{"edge-orchestrator.intel.com/project-id": activeProjectID, "default-extension": "baseline", "example-key": "user-labels"}
var exampleCluster = capi.Cluster{
	TypeMeta: metav1.TypeMeta{},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "example-cluster",
		Labels:    clusterLabels,
		Namespace: activeProjectID,
	},
	Spec: capi.ClusterSpec{
		Paused:               false,
		ClusterNetwork:       &capi.ClusterNetwork{},
		ControlPlaneEndpoint: capi.APIEndpoint{},
		ControlPlaneRef:      &v1.ObjectReference{},
		InfrastructureRef:    &v1.ObjectReference{},
		Topology: &capi.Topology{
			Version: "v1.21.1",
			Class:   "baseline",
		},
	},
	Status: capi.ClusterStatus{Phase: "Provisioned"},
}

var exampleDetails = api.ClusterDetailInfo{
	KubernetesVersion: ptr("v1.21.1"),
	Labels: &map[string]interface{}{
		"default-extension": "baseline",
		"example-key":       "user-labels",
	},
	Name: ptr("example-cluster"),
	LifecyclePhase: &api.GenericStatus{
		Indicator: statusIndicatorPtr("STATUS_INDICATION_UNSPECIFIED"),
		Message:   ptr("Condition not found"),
		Timestamp: ptr(uint64(0)),
	},
	NodeHealth: &api.GenericStatus{
		Indicator: statusIndicatorPtr("STATUS_INDICATION_ERROR"),
		Message:   ptr("nodes are unhealthy (0/1);[MachinePhase ]"),
		Timestamp: ptr(uint64(0)),
	},
	Nodes: &[]api.NodeInfo{
		{
			Id:   &hostId,
			Role: ptr("all"),
			Status: &api.StatusInfo{
				Condition: statusStatusInfoConditionPtr("STATUS_CONDITION_UNKNOWN"),
				Reason:    ptr("Unknown"),
			},
		},
	},
	ProviderStatus: &api.GenericStatus{
		Indicator: statusIndicatorPtr("STATUS_INDICATION_UNSPECIFIED"),
		Message:   ptr("condition not found"),
		Timestamp: ptr(uint64(0)),
	},
	Template: ptr("baseline"),
	ControlPlaneReady: &api.GenericStatus{
		Indicator: statusIndicatorPtr("STATUS_INDICATION_UNSPECIFIED"),
		Message:   ptr("condition not found"),
		Timestamp: ptr(uint64(0)),
	},
	InfrastructureReady: &api.GenericStatus{
		Indicator: statusIndicatorPtr("STATUS_INDICATION_UNSPECIFIED"),
		Message:   ptr("condition not found"),
		Timestamp: ptr(uint64(0)),
	},
}

func TestGetV2ClustersClusterDetail200(t *testing.T) {
	t.Run("successful cluster details retrieval", func(t *testing.T) {
		nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"

		// Create a machine with the correct InfrastructureRef that matches the IntelMachine name
		machine, err := convert.ToUnstructured(capi.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "example-machine", Namespace: activeProjectID},
			TypeMeta:   metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"},
			Spec: capi.MachineSpec{
				ClusterName: "example-cluster",
				InfrastructureRef: v1.ObjectReference{
					Name:      "example-intelmachine", // Must match the IntelMachine name exactly
					Namespace: activeProjectID,
					Kind:      "IntelMachine", // Make sure the Kind matches
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", // Add the API version
				},
			},
			Status: capi.MachineStatus{
				NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)},
			},
		})
		require.Nil(t, err)

		// Create an IntelMachine with the exact same name as referenced in the machine
		intelMachineObj := intelProvider.IntelMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "example-intelmachine", // Same name as in the machine's InfrastructureRef
				Namespace:   activeProjectID,
				Annotations: map[string]string{"intelmachine.infrastructure.cluster.x-k8s.io/host-id": hostId},
			},
			TypeMeta: metav1.TypeMeta{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
				Kind:       "IntelMachine",
			},
			Spec: intelProvider.IntelMachineSpec{},
		}
		require.Nil(t, err)

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock the ListCached call for Machine resource
		mockK8sClient.EXPECT().ListCached(mock.Anything, core.MachineResourceSchema, activeProjectID, mock.Anything).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil)
		// Mock the Cluster call
		mockK8sClient.EXPECT().GetCluster(mock.Anything, activeProjectID, "example-cluster").Return(&exampleCluster, nil)

		// Add mock for Machines call that's being made
		machineObj := capi.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-machine",
				Namespace: activeProjectID,
			},
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "Machine",
			},
			Spec: capi.MachineSpec{
				ClusterName: "example-cluster",
				InfrastructureRef: v1.ObjectReference{
					Name:      "example-intelmachine",
					Namespace: activeProjectID,
					Kind:      "IntelMachine",
				},
			},
			Status: capi.MachineStatus{
				Phase: string(capi.MachinePhaseUnknown),
			},
		}
		mockK8sClient.EXPECT().GetMachines(mock.Anything, activeProjectID, "example-cluster").Return([]capi.Machine{machineObj}, nil)

		// We may need to mock additional GetCached calls for other resources like MachineDeployment
		// Mock any additional GetCached or ListCached calls with mock.Anything matchers to catch unexpected calls
		mockK8sClient.EXPECT().GetCached(mock.Anything, mock.Anything, activeProjectID, mock.Anything).Return(nil, nil).Maybe()
		
		mockK8sClient.EXPECT().ListCached(mock.Anything, mock.Anything, activeProjectID, mock.Anything).Return(&unstructured.UnstructuredList{}, nil).Maybe()
		
		mockK8sClient.EXPECT().IntelMachine(mock.Anything, activeProjectID,"example-intelmachine").Return(intelMachineObj, nil)
		// Create a new server with the mocked k8s client
		server := NewServer(mockK8sClient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/clusterdetail", nodeId), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// handle the request and check response
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)
		
		// Debug output in case of failure
		if rr.Code != http.StatusOK {
			t.Logf("Response body: %s", rr.Body.String())
		}
		
		assert.Equal(t, http.StatusOK, rr.Code)

		// Convert exampleDetails to JSON string
		expectedResponse, err := json.Marshal(exampleDetails)
		require.Nil(t, err)

		// Compare the JSON string with rr.Body
		assert.JSONEq(t, string(expectedResponse), rr.Body.String())
	})
}

func TestGetV2ClustersClusterDetail404(t *testing.T) {
	tests := []struct {
		name             string
		nodeId           string
		activeProjectID  string
		mockSetup        func(mockK8sClient *k8s.MockClient)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:            "no machine",
			nodeId:          "64e797f6-db22-445e-b606-4228d4f1c2bd",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				mockK8sClient.EXPECT().ListCached(
					mock.Anything,
					core.MachineResourceSchema,
					activeProjectID,
					metav1.ListOptions{},
				).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: "{\"message\":\"machine not found\"}\n",
		},
		{
			name:            "wrong machine",
			nodeId:          "94e797f6-db22-445e-b606-4228d4f1c2bd",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"
				machine, err := convert.ToUnstructured(capi.Machine{
					TypeMeta: metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"},
					Status:   capi.MachineStatus{NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)}},
				})
				require.Nil(t, err)
				mockK8sClient.EXPECT().ListCached(
					mock.Anything,
					core.MachineResourceSchema,
					activeProjectID,
					metav1.ListOptions{},
				).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: "{\"message\":\"machine not found\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client and set up mocks
			mockK8sClient := k8s.NewMockClient(t)
			tt.mockSetup(mockK8sClient)

			// Create server with the mock client
			server := NewServer(mockK8sClient)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			// Create request and response recorder
			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/clusterdetail", tt.nodeId), nil)
			req.Header.Set("Activeprojectid", activeProjectID)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Configure handler and serve request
			handler, err := server.ConfigureHandler()
			require.Nil(t, err)
			handler.ServeHTTP(rr, req)

			// Verify response
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func createGetV2ClusterDetailStubServer(t *testing.T) *Server {
	mockK8sClient := k8s.NewMockClient(t)
	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.MachineResourceSchema,
		mock.Anything,
		metav1.ListOptions{},
	).Return(&unstructured.UnstructuredList{}, nil).Maybe()

	return &Server{
		k8sclient: mockK8sClient,
	}
}

func FuzzGetV2ClusterDetail(f *testing.F) {
	f.Add("abc",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, nodeId string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2ClusterDetailStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.GetV2ClustersNodeIdClusterdetailRequestObject{
			NodeId: nodeId,
			Params: api.GetV2ClustersNodeIdClusterdetailParams{
				Activeprojectid: activeprojectid,
			},
		}
		_, _ = server.GetV2ClustersNodeIdClusterdetail(context.Background(), req)
	})
}

func statusStatusInfoConditionPtr(indicator api.StatusInfoCondition) *api.StatusInfoCondition {
	return &indicator
}
