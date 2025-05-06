// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func setupMockServer(t *testing.T, expectedCluster capi.Cluster, expectedActiveProjectID string, getReturn *unstructured.Unstructured, getError error) *Server {
	nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"
	machine, err := convert.ToUnstructured(capi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-machine",
			Namespace: expectedActiveProjectID,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1beta1",
			Kind:       "Machine",
		},
		Spec: capi.MachineSpec{
			ClusterName: expectedCluster.Name,
			InfrastructureRef: v1.ObjectReference{
				Name:       "example-intelmachine",
				Namespace:  expectedActiveProjectID,
				Kind:       "IntelMachine",
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			},
		},
		Status: capi.MachineStatus{
			NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)},
			Phase:   string(capi.MachinePhaseUnknown),
		},
	})
	require.Nil(t, err)

	// Create an IntelMachine with the exact same name as referenced in the machine
	intelMachineObj := intelProvider.IntelMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "example-intelmachine",
			Namespace:   expectedActiveProjectID,
			Annotations: map[string]string{"intelmachine.infrastructure.cluster.x-k8s.io/host-id": hostId},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			Kind:       "IntelMachine",
		},
		Spec: intelProvider.IntelMachineSpec{},
	}
	intelmachine, err := convert.ToUnstructured(intelMachineObj)
	require.Nil(t, err)

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock ListCached for machines
	mockK8sClient.EXPECT().ListCached(mock.Anything, core.MachineResourceSchema, expectedActiveProjectID, mock.Anything).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil).Maybe()

	// Mock GetCached for IntelMachine resource - This is important for resolving the provider
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		k8s.IntelMachineResourceSchema,
		expectedActiveProjectID,
		"example-intelmachine",
	).Return(intelmachine, nil).Maybe()

	// Add IntelMachine call mock - this is what was missing
	mockK8sClient.EXPECT().IntelMachine(mock.Anything, expectedActiveProjectID, "example-intelmachine").Return(intelMachineObj, nil).Maybe()

	// Mock Cluster method
	if getError != nil {
		mockK8sClient.EXPECT().GetCluster(mock.Anything, expectedActiveProjectID, expectedCluster.Name).Return(nil, getError)
	} else {
		mockK8sClient.EXPECT().GetCluster(mock.Anything, expectedActiveProjectID, mock.Anything).Return(&expectedCluster, nil).Maybe()
	}
	// Mock Machines method
	machineObj := capi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-machine",
			Namespace: expectedActiveProjectID,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1beta1",
			Kind:       "Machine",
		},
		Spec: capi.MachineSpec{
			ClusterName: expectedCluster.Name,
			InfrastructureRef: v1.ObjectReference{
				Name:       "example-intelmachine",
				Namespace:  expectedActiveProjectID,
				Kind:       "IntelMachine",
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			},
		},
		Status: capi.MachineStatus{
			Phase: string(capi.MachinePhaseUnknown),
		},
	}
	mockK8sClient.EXPECT().GetMachines(mock.Anything, expectedActiveProjectID, expectedCluster.Name).Return([]capi.Machine{machineObj}, nil).Maybe()
	// Create a new server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	return server
}

func TestGetV2Cluster200(t *testing.T) {
	// prepare test data
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	expectedCluster := capi.Cluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-cluster",
			Namespace: "655a6892-4280-4c37-97b1-31161ac0b99e",
			Labels: map[string]string{"edge-orchestrator.intel.com/project-id": "655a6892-4280-4c37-97b1-31161ac0b99e",
				"default-extension": "baseline"}},
		Spec: capi.ClusterSpec{
			Paused:               false,
			ClusterNetwork:       &capi.ClusterNetwork{},
			ControlPlaneEndpoint: capi.APIEndpoint{},
			ControlPlaneRef:      &v1.ObjectReference{},
			InfrastructureRef:    &v1.ObjectReference{},
			Topology:             &capi.Topology{Version: "v1.21.1", Class: "baseline"},
		},
		Status: capi.ClusterStatus{
			Phase: "Provisioned",
			Conditions: []capi.Condition{
				{
					Type:               capi.ConditionType("Ready"),
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Message:            "active",
				},
				{
					Type:               capi.ConditionType("ControlPlaneReady"),
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Message:            "ready",
				},
				{
					Type:               capi.ConditionType("InfrastructureReady"),
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Message:            "ready",
				},
			},
		},
	}

	expectedClusterDetailInfo := api.ClusterDetailInfo{
		KubernetesVersion: ptr("v1.21.1"),
		Labels: &map[string]interface{}{
			"default-extension": "baseline",
		},
		LifecyclePhase: &api.GenericStatus{
			Indicator: (*api.StatusIndicator)(ptr("STATUS_INDICATION_IDLE")),
			Message:   ptr("active"),
			Timestamp: uint64Ptr(uint64(metav1.Now().Unix())),
		},
		Name: ptr("example-cluster"),
		NodeHealth: &api.GenericStatus{
			Indicator: (*api.StatusIndicator)(ptr("STATUS_INDICATION_ERROR")),
			Message:   ptr("nodes are unhealthy (0/1);[MachinePhase Unknown]"),
			Timestamp: uint64Ptr(uint64(metav1.Now().Unix())),
		},
		Nodes: &[]api.NodeInfo{{
			Id:   &hostId,
			Role: ptr("all"),
			Status: &api.StatusInfo{
				Condition: statusStatusInfoConditionPtr("STATUS_CONDITION_UNKNOWN"),
				Reason:    ptr("Unknown"),
			},
		}},
		ProviderStatus: &api.GenericStatus{
			Indicator: (*api.StatusIndicator)(ptr("STATUS_INDICATION_IDLE")),
			Message:   ptr("ready"),
			Timestamp: uint64Ptr(uint64(metav1.Now().Unix())),
		},
		ControlPlaneReady: &api.GenericStatus{
			Indicator: (*api.StatusIndicator)(ptr("STATUS_INDICATION_IDLE")),
			Message:   ptr("ready"),
			Timestamp: uint64Ptr(uint64(metav1.Now().Unix())),
		},
		InfrastructureReady: &api.GenericStatus{
			Indicator: (*api.StatusIndicator)(ptr("STATUS_INDICATION_IDLE")),
			Message:   ptr("ready"),
			Timestamp: uint64Ptr(uint64(metav1.Now().Unix())),
		},
		Template: ptr("baseline"),
	}

	// Convert the expected cluster to an unstructured object
	unstructuredCluster, err := convert.ToUnstructured(expectedCluster)
	require.NoError(t, err, "Failed to convert cluster to unstructured")

	server := setupMockServer(t, expectedCluster, expectedActiveProjectID, unstructuredCluster, nil)

	// Create a new request & response recorder
	req := httptest.NewRequest("GET", "/v2/clusters/example-cluster", nil)
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	rr := httptest.NewRecorder()

	// create a handler with middleware to serve the request
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)

	// Check the response status
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)

	// Parse the response body
	var actualClusterDetailInfo api.ClusterDetailInfo
	err = json.Unmarshal(rr.Body.Bytes(), &actualClusterDetailInfo)
	require.NoError(t, err, "Failed to unmarshal response body")

	// Compare the actual response with the expected response
	require.Equal(t, expectedClusterDetailInfo, actualClusterDetailInfo, "Response body = %v, want %v", actualClusterDetailInfo, expectedClusterDetailInfo)
}

func TestGetV2Cluster500(t *testing.T) {
	// prepare test data
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	expectedCluster := capi.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "example-cluster", Namespace: expectedActiveProjectID}}
	unstructuredCluster, _ := convert.ToUnstructured(expectedCluster)

	t.Run("InternalServerError", func(t *testing.T) {
		// Setup the mock server for the first part of the test
		server := setupMockServer(t, expectedCluster, expectedActiveProjectID, unstructuredCluster, nil)

		// create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/clusters/example-cluster", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// check the response status
		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)
	})

	t.Run("ConvertError", func(t *testing.T) {
		// Setup the mock server for the second part of the test
		server := setupMockServer(t, expectedCluster, expectedActiveProjectID, &unstructured.Unstructured{}, nil)

		// Create a new request object
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: uuid.MustParse(expectedActiveProjectID),
			},
			Name: "example-cluster",
		}

		// Call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)

		// Check the response
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)
		require.IsType(t, api.GetV2ClustersName500JSONResponse{}, response, "GetV2ClustersName() response = %v, want %v", response, api.GetV2ClustersName500JSONResponse{})
	})

	t.Run("MissingClusterName", func(t *testing.T) {
		// Setup the mock server for the third part of the test
		expectedCluster.Name = ""
		server := setupMockServer(t, expectedCluster, expectedActiveProjectID, &unstructured.Unstructured{}, nil)

		// Create a new request object
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: uuid.MustParse(expectedActiveProjectID),
			},
			Name: "example-cluster",
		}

		// Call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)

		// Check the response
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)
		require.IsType(t, api.GetV2ClustersName500JSONResponse{}, response, "GetV2ClustersName() response = %v, want %v", response, api.GetV2ClustersName500JSONResponse{})

		// Check the response message
		resp := response.(api.GetV2ClustersName500JSONResponse)
		require.Equal(t, "missing cluster name", *resp.N500InternalServerErrorJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N500InternalServerErrorJSONResponse.Message, "missing cluster name")
	})
}

func TestGetV2ClustersName400(t *testing.T) {
	// Create a new server instance with a nil k8sClient since it won't be used in this test
	server := NewServer(nil)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	t.Run("MissingActiveProjectID", func(t *testing.T) {
		// create a new request object with an empty active project ID
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: uuid.UUID{},
			},
			Name: "example-cluster",
		}

		// call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)

		// check the response message
		resp := response.(api.GetV2ClustersName400JSONResponse)
		require.Equal(t, "no active project id provided", *resp.N400BadRequestJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N400BadRequestJSONResponse.Message, "no active project id provided")
	})

	t.Run("InvalidActiveProjectID", func(t *testing.T) {
		// create a new request object with an invalid active project ID (all zeros)
		invalidUUID := uuid.UUID{}
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: invalidUUID,
			},
			Name: "example-cluster",
		}

		// Manually set an invalid UUID string
		request.Params.Activeprojectid = uuid.UUID{}

		// call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)

		// check the response message
		resp := response.(api.GetV2ClustersName400JSONResponse)
		require.Equal(t, "no active project id provided", *resp.N400BadRequestJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N400BadRequestJSONResponse.Message, "no active project id provided")
	})

	t.Run("MissingClusterName", func(t *testing.T) {
		// parse the UUID string to uuid.UUID
		activeProjectID, err := uuid.Parse("655a6892-4280-4c37-97b1-31161ac0b99e")
		require.NoError(t, err, "uuid.Parse() error = %v, want nil", err)

		// create a new request object with an empty cluster name
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: activeProjectID,
			},
			Name: "",
		}

		// call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)

		// check the response message
		resp := response.(api.GetV2ClustersName400JSONResponse)
		require.Equal(t, "cluster name is required", *resp.N400BadRequestJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N400BadRequestJSONResponse.Message, "cluster name is required")
	})

	t.Run("InvalidClusterNameFormat", func(t *testing.T) {
		// create a new request object with a valid active project ID but invalid cluster name
		validUUID := uuid.New()
		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: validUUID,
			},
			Name: "invalid_cluster_name!", // Invalid name with special characters
		}

		// call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)

		// check the response message
		resp := response.(api.GetV2ClustersName400JSONResponse)
		require.Equal(t, "invalid cluster name format", *resp.N400BadRequestJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N400BadRequestJSONResponse.Message, "invalid cluster name format")
	})
}

func TestGetV2ClustersName404(t *testing.T) {
	// Define the expected cluster and project ID
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	expectedCluster := capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-cluster", Namespace: expectedActiveProjectID},
	}
	server := setupMockServer(t, expectedCluster, expectedActiveProjectID, nil, k8s.ErrClusterNotFound)

	t.Run("WrongClusterName", func(t *testing.T) {
		// create a new request object with a valid active project ID
		activeProjectUUID, err := uuid.Parse(expectedActiveProjectID)
		require.NoError(t, err, "uuid.Parse() error = %v, want nil")

		request := api.GetV2ClustersNameRequestObject{
			Params: api.GetV2ClustersNameParams{
				Activeprojectid: activeProjectUUID,
			},
			Name: "wrong-cluster",
		}

		// call the GetV2ClustersName function
		response, err := server.GetV2ClustersName(context.Background(), request)
		require.NoError(t, err, "GetV2ClustersName() error = %v, want nil", err)

		// check the response type and message
		resp, ok := response.(api.GetV2ClustersName404JSONResponse)
		require.True(t, ok, "GetV2ClustersName() response type = %T, want api.GetV2ClustersName404JSONResponse", response)
		require.Equal(t, "failed to get cluster, err: cluster not found", *resp.N404NotFoundJSONResponse.Message, "GetV2ClustersName() message = %v, want %v", *resp.N404NotFoundJSONResponse.Message, "cluster not found")
	})
}

func createGetV2ClustersNameStubServer(t *testing.T) *Server {
	// Create a mock k8s client
	mockK8sClient := k8s.NewMockClient(t)

	// Setting up minimal mocks for the fuzzing test
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&unstructured.Unstructured{}, nil).Maybe()

	mockK8sClient.EXPECT().GetCluster(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&capi.Cluster{}, nil).Maybe()

	mockK8sClient.EXPECT().GetMachines(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]capi.Machine{}, nil).Maybe()

	return &Server{
		k8sclient: mockK8sClient,
	}
}

func FuzzGetV2NameClusters(f *testing.F) {
	f.Add("abc", byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2ClustersNameStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.GetV2ClustersNameParams{
			Activeprojectid: activeprojectid,
		}
		req := api.GetV2ClustersNameRequestObject{
			Name:   name,
			Params: params,
		}
		_, _ = server.GetV2ClustersName(context.Background(), req)
	})
}
