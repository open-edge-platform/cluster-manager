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

	"github.com/open-edge-platform/cluster-manager/internal/convert"
	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
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

		machine, err := convert.ToUnstructured(capi.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "example-machine", Namespace: activeProjectID},
			TypeMeta:   metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"},
			Spec:       capi.MachineSpec{InfrastructureRef: v1.ObjectReference{Name: "example-infrastructure", Kind: "IntelMachine", Namespace: activeProjectID}},
			Status:     capi.MachineStatus{NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)}}})
		require.Nil(t, err)

		intelmachine, err := convert.ToUnstructured(intelProvider.IntelMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "example-intelmachine", Namespace: activeProjectID, Annotations: map[string]string{"intelmachine.infrastructure.cluster.x-k8s.io/host-id": hostId}},
			TypeMeta:   metav1.TypeMeta{APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1", Kind: "IntelMachine"},
			Spec:       intelProvider.IntelMachineSpec{},
		})
		require.Nil(t, err)

		// mock the machine resource
		machineResource := k8s.NewMockResourceInterface(t)
		machineResource.EXPECT().List(mock.Anything, mock.Anything).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil).Maybe()
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(machineResource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsResource)

		// mock the intelmachine resource
		intelMachineResource := k8s.NewMockResourceInterface(t)
		intelMachineResource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(intelmachine, nil)
		nsIntelMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsIntelMachineResource.EXPECT().Namespace(activeProjectID).Return(intelMachineResource)
		mockedk8sclient.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(nsIntelMachineResource)

		// mock the cluster resource
		clusterObject, err := convert.ToUnstructured(exampleCluster)
		require.Nil(t, err)
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(clusterObject, nil)
		clusterNsResource := k8s.NewMockNamespaceableResourceInterface(t)
		clusterNsResource.EXPECT().Namespace(activeProjectID).Return(clusterResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(clusterNsResource)

		// Create a new server with the mocked k8s client
		server := NewServer(mockedk8sclient)
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
		mockSetup        func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:            "no machine",
			nodeId:          "64e797f6-db22-445e-b606-4228d4f1c2bd",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				machineResource := k8s.NewMockResourceInterface(t)
				machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil)
				nsResource.EXPECT().Namespace(activeProjectID).Return(machineResource)

				mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsResource)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: "{\"message\":\"machine not found\"}\n",
		},
		{
			name:            "wrong machine",
			nodeId:          "94e797f6-db22-445e-b606-4228d4f1c2bd",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"
				machineResource := k8s.NewMockResourceInterface(t)
				machine, err := convert.ToUnstructured(capi.Machine{TypeMeta: metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"}, Status: capi.MachineStatus{NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)}}})
				require.Nil(t, err)
				machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil)
				nsResource.EXPECT().Namespace(activeProjectID).Return(machineResource)

				mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsResource)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: "{\"message\":\"machine not found\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// set up mock resources and server
			mockedk8sclient := k8s.NewMockInterface(t)
			resource := k8s.NewMockResourceInterface(t)
			nsResource := k8s.NewMockNamespaceableResourceInterface(t)
			tt.mockSetup(resource, nsResource, mockedk8sclient)
			server := NewServer(mockedk8sclient)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")
			// create request and response recorder
			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/clusterdetail", tt.nodeId), nil)
			req.Header.Set("Activeprojectid", activeProjectID)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler, err := server.ConfigureHandler()
			require.Nil(t, err)
			// serve the request and check response
			handler.ServeHTTP(rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())

		})
	}
}
func TestGetV2ClustersClusterDetail400(t *testing.T) {
	tests := []struct {
		name             string
		activeProjectID  string
		expectedCode     int
		expectedResponse string
	}{
		{
			name:             "no active projectId",
			activeProjectID:  "",
			expectedCode:     http.StatusBadRequest,
			expectedResponse: `{"message": "no active project id provided"}`,
		},
		{
			name:             "zero values for projectId",
			activeProjectID:  "00000000-0000-0000-0000-000000000000",
			expectedCode:     http.StatusBadRequest,
			expectedResponse: `{"message":"no active project id provided"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"
			server := NewServer(nil)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/clusterdetail", nodeId), nil)
			if tt.activeProjectID != "" {
				req.Header.Set("Activeprojectid", tt.activeProjectID)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func createGetV2ClusterDetailStubServer(t *testing.T) *Server {
	machineResource := k8s.NewMockResourceInterface(t)
	machineResource.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&unstructured.UnstructuredList{}, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(machineResource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
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
