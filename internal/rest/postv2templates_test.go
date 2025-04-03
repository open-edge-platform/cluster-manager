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

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// TestPostV2Templates200 tests the PostV2Templates handler with a 200 response
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

	clusterTemplate, err := template.FromTemplateInfoToClusterTemplate(templateInfo)
	require.NoError(t, err, "FromTemplateInfoToClusterTemplate() error = %v, want nil")

	unstructuredClusterTemplate, err := convert.ToUnstructured(&clusterTemplate)
	require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")

	// configure mockery to return the response object on a Create() call
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Create(mock.Anything, unstructuredClusterTemplate, v1.CreateOptions{}).Return(nil, nil)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	// create a new server with the mocked mockedk8sclient
	server := NewServer(mockedk8sclient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// serve the request
	handler.ServeHTTP(rr, req)

	// check the response status
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

			mockedk8sclient := k8s.NewMockInterface(t)
			server := NewServer(mockedk8sclient)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			// create a handler with middleware
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

	// configure mockery to return a conflict error on a Create() call
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Create(mock.Anything, mock.Anything, v1.CreateOptions{}).Return(nil, &errors.StatusError{
		ErrStatus: v1.Status{
			Status:  v1.StatusFailure,
			Code:    http.StatusConflict,
			Reason:  v1.StatusReasonAlreadyExists,
			Message: "ClusterTemplate already exists",
		},
	})
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	// create a new server with the mocked mockedk8sclient
	server := NewServer(mockedk8sclient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// serve the request
	handler.ServeHTTP(rr, req)

	// check the response status
	require.Equal(t, http.StatusConflict, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusConflict)

	// check the response body
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

	// configure mockery to return an internal server error on a Create() call
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Create(mock.Anything, mock.Anything, v1.CreateOptions{}).Return(nil, &errors.StatusError{
		ErrStatus: v1.Status{
			Status:  v1.StatusFailure,
			Code:    http.StatusInternalServerError,
			Reason:  v1.StatusReasonInternalError,
			Message: "Internal server error",
		},
	})
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	// create a new server with the mocked mockedk8sclient
	server := NewServer(mockedk8sclient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// create a new request & response recorder
	body, err := json.Marshal(templateInfo)
	require.NoError(t, err, "json.Marshal() error = %v, want nil")
	req := httptest.NewRequest("POST", "/v2/templates", bytes.NewReader(body))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// serve the request
	handler.ServeHTTP(rr, req)

	// check the response status
	require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

	// check the response body
	var respbody api.PostV2Templates500JSONResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respbody)
	require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
	expectedErrorMessage := "Internal server error"
	require.Contains(t, *respbody.Message, expectedErrorMessage, "ServeHTTP() body = %v, want %v", *respbody.Message, expectedErrorMessage)
}

func createPostV2TemplatesStubServer(t *testing.T) *Server {
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Create(mock.Anything, mock.Anything, v1.CreateOptions{}).Return(nil, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzPostV2Templates(f *testing.F) {
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
