// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// TestPostV2Templates200 test fix
func TestPostV2Templates200(t *testing.T) {
	var expectedActiveProjectID = "655a6892-4280-4c37-97b1-31161ac0b99e"

	cptype := api.Kubeadm
	infratype := api.Docker

	templateInfo := api.TemplateInfo{
		Name:                     "test",
		Version:                  "v1.0.0",
		Controlplaneprovidertype: &cptype,
		Infraprovidertype:        &infratype,
		KubernetesVersion:        "v1.21.0",
	}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock the full chain of Dynamic -> Resource -> Namespace -> Create calls
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	// Setup the mock chain
	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface)
	mockDynamicInterface.EXPECT().Resource(core.TemplateResourceSchema).Return(mockNamespaceableResourceInterface)
	mockNamespaceableResourceInterface.EXPECT().Namespace(expectedActiveProjectID).Return(mockResourceInterface)
	mockResourceInterface.EXPECT().Create(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil, nil)

	// Create a new server with the mocked client
	server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check the response status
	require.Equal(t, http.StatusCreated, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 201)
}

func TestPostV2Templates400(t *testing.T) {
	testCases := []struct {
		name          string
		projectId     string
		templateInfo  api.TemplateInfo
		expectedError string
	}{
		{
			name:      "bad activeprojectid",
			projectId: "00000000-0000-0000-0000-000000000000",
			templateInfo: api.TemplateInfo{
				Name:                     "test",
				Version:                  "v1.0.0",
				Controlplaneprovidertype: ptr(api.Kubeadm),
				Infraprovidertype:        ptr(api.Docker),
				KubernetesVersion:        "v1.21.0",
			},
			expectedError: "no active project id provided",
		},
		{
			name:      "missing name",
			projectId: expectedActiveProjectID,
			templateInfo: api.TemplateInfo{
				Version:                  "v1.0.0",
				Controlplaneprovidertype: ptr(api.Kubeadm),
				Infraprovidertype:        ptr(api.Docker),
				KubernetesVersion:        "v1.21.0",
			},
			expectedError: "Error at \"/name\"",
		},
		{
			name:      "missing version",
			projectId: expectedActiveProjectID,
			templateInfo: api.TemplateInfo{
				Name:                     "test",
				Controlplaneprovidertype: ptr(api.Kubeadm),
				Infraprovidertype:        ptr(api.Docker),
				KubernetesVersion:        "v1.21.0",
			},
			expectedError: "Error at \"/version\"",
		},
		{
			name:      "invalid control plane provider type",
			projectId: expectedActiveProjectID,
			templateInfo: api.TemplateInfo{
				Name:                     "test",
				Version:                  "v1.0.0",
				Controlplaneprovidertype: ptr(api.TemplateInfoControlplaneprovidertype("invalid")),
				Infraprovidertype:        ptr(api.Docker),
				KubernetesVersion:        "v1.21.0",
			},
			expectedError: "Error at \"/controlplaneprovidertype\"",
		},
		{
			name:      "invalid infra provider type",
			projectId: expectedActiveProjectID,
			templateInfo: api.TemplateInfo{
				Name:              "test",
				Version:           "v1.0.0",
				Infraprovidertype: ptr(api.TemplateInfoInfraprovidertype("invalid")),
				KubernetesVersion: "v1.21.0",
			},
			expectedError: " Error at \"/infraprovidertype\"",
		},
		{
			name:      "missing Kubernetes version",
			projectId: expectedActiveProjectID,
			templateInfo: api.TemplateInfo{
				Name:                     "test",
				Version:                  "v1.0.0",
				Controlplaneprovidertype: ptr(api.Kubeadm),
				Infraprovidertype:        ptr(api.Docker),
			},
			expectedError: " Error at \"/kubernetesVersion\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.templateInfo)
			require.NoError(t, err, "json.Marshal() error = %v, want nil")
			req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
			req.Header.Set("Activeprojectid", tc.projectId)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Use NewMockClient
			mockK8sClient := k8s.NewMockClient(t)
			server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			// Create a handler with middleware
			handler, err := server.ConfigureHandler()
			require.Nil(t, err)
			handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)

			var respbody api.PostV2Templates400JSONResponse
			err = json.Unmarshal(rr.Body.Bytes(), &respbody)
			require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
			require.Contains(t, *respbody.Message, tc.expectedError, "ServeHTTP() body = %v, want %v", *respbody.Message, tc.expectedError)
		})
	}
}

func TestPostV2Templates409(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	cptype := api.Kubeadm
	infratype := api.Docker

	templateInfo := api.TemplateInfo{
		Name:                     "test",
		Version:                  "v1.0.0",
		Controlplaneprovidertype: &cptype,
		Infraprovidertype:        &infratype,
		KubernetesVersion:        "v1.21.0",
	}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock the full chain of Dynamic -> Resource -> Namespace -> Create calls
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	// Setup the mock chain
	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface)
	mockDynamicInterface.EXPECT().Resource(core.TemplateResourceSchema).Return(mockNamespaceableResourceInterface)
	mockNamespaceableResourceInterface.EXPECT().Namespace(expectedActiveProjectID).Return(mockResourceInterface)

	// Return a conflict error when trying to create the resource
	mockResourceInterface.EXPECT().Create(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil, &errors.StatusError{
		ErrStatus: v1.Status{
			Status:  v1.StatusFailure,
			Code:    http.StatusConflict,
			Reason:  v1.StatusReasonAlreadyExists,
			Message: "ClusterTemplate already exists",
		},
	})

	// Create a new server with the mocked client
	server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check the response status
	require.Equal(t, http.StatusConflict, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusConflict)

	// Check the response body
	var respbody api.PostV2Templates409JSONResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respbody)
	require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
	expectedErrorMessage := "already exists"
	require.Contains(t, *respbody.Message, expectedErrorMessage, "ServeHTTP() body = %v, want %v", *respbody.Message, expectedErrorMessage)
}

func TestPostV2Templates500(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	cptype := api.Kubeadm
	infratype := api.Docker

	templateInfo := api.TemplateInfo{
		Name:                     "test",
		Version:                  "v1.0.0",
		Controlplaneprovidertype: &cptype,
		Infraprovidertype:        &infratype,
		KubernetesVersion:        "v1.21.0",
	}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock the full chain of Dynamic -> Resource -> Namespace -> Create calls
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	// Setup the mock chain
	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface)
	mockDynamicInterface.EXPECT().Resource(core.TemplateResourceSchema).Return(mockNamespaceableResourceInterface)
	mockNamespaceableResourceInterface.EXPECT().Namespace(expectedActiveProjectID).Return(mockResourceInterface)

	// Return an internal server error when trying to create the resource
	mockResourceInterface.EXPECT().Create(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil, &errors.StatusError{
		ErrStatus: v1.Status{
			Status:  v1.StatusFailure,
			Code:    http.StatusInternalServerError,
			Reason:  v1.StatusReasonInternalError,
			Message: "Internal server error",
		},
	})

	// Create a new server with the mocked client
	server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check the response status
	require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

	// Check the response body
	var respbody api.PostV2Templates500JSONResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respbody)
	require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
	expectedErrorMessage := "Internal server error"
	require.Contains(t, *respbody.Message, expectedErrorMessage, "ServeHTTP() body = %v, want %v", *respbody.Message, expectedErrorMessage)
}

func createPostV2TemplatesStubServer(t *testing.T) *Server {
	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock the dynamic interface for fuzzing
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	// Setup flexible mocks for fuzzing
	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface).Maybe()
	mockDynamicInterface.EXPECT().Resource(mock.Anything).Return(mockNamespaceableResourceInterface).Maybe()
	mockNamespaceableResourceInterface.EXPECT().Namespace(mock.Anything).Return(mockResourceInterface).Maybe()
	mockResourceInterface.EXPECT().Create(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil, nil).Maybe()

	// Mock CreateTemplate method for fuzzing tests
	mockK8sClient.EXPECT().CreateTemplate(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil).Maybe()

	// Create and return the server
	return NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
}

func FuzzPostV2Templates(f *testing.F) {
	// No changes needed here - just using the updated stub server
	f.Add("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, labelKey, labelVal, podsCidrStr, servicesCidrStr,
		clusterCfgStr, ctrlPlaneProvStr, desc, infraProvStr, k8sVer, name, ver string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createPostV2TemplatesStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.PostV2TemplatesParams{
			Activeprojectid: activeprojectid,
		}
		clusterLabels := map[string]string{labelKey: labelVal}
		pods := api.NetworkRanges{
			CidrBlocks: []string{podsCidrStr},
		}
		services := api.NetworkRanges{
			CidrBlocks: []string{servicesCidrStr},
		}
		clusterNetwork := api.ClusterNetwork{
			Pods:     &pods,
			Services: &services,
		}
		clusterConfig := map[string]interface{}{clusterCfgStr: nil}
		ctrlPlaneProv := api.TemplateInfoControlplaneprovidertype(ctrlPlaneProvStr)
		infraProv := api.TemplateInfoInfraprovidertype(infraProvStr)
		templateInfo := api.TemplateInfo{
			ClusterLabels:            &clusterLabels,
			ClusterNetwork:           &clusterNetwork,
			Clusterconfiguration:     &clusterConfig,
			Controlplaneprovidertype: &ctrlPlaneProv,
			Description:              &desc,
			Infraprovidertype:        &infraProv,
			KubernetesVersion:        k8sVer,
			Name:                     name,
			Version:                  ver,
		}
		body := api.PostV2TemplatesJSONRequestBody(templateInfo)
		req := api.PostV2TemplatesRequestObject{
			Params: params,
			Body:   &body,
		}
		_, _ = server.PostV2Templates(context.Background(), req)
	})
}
