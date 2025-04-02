// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createMockServerTemplateNameVersion(t *testing.T, template v1alpha1.ClusterTemplate, activeProjectId string, getError error) *Server {
	unstructuredTemplate, err := convert.ToUnstructured(template)
	require.NoError(t, err, "convertAnyToUnstructured() error = %v, want nil")

	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(unstructuredTemplate, getError)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(activeProjectId).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	return NewServer(mockedk8sclient)
}

func TestGetV2TemplatesNameVersion(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	tests := []struct {
		name                 string
		template             v1alpha1.ClusterTemplate
		templateName         string
		templateVersion      string
		expectedTemplateInfo api.TemplateInfo
		expectedError        error
		expectedStatusCode   int
		expectedErrMessage   string
	}{
		{
			name:                 "200 OK",
			template:             template1,
			templateName:         "foo",
			templateVersion:      "v1.0.0",
			expectedTemplateInfo: templateInfo1,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:            "400 No Template Found",
			template:        v1alpha1.ClusterTemplate{},
			templateName:    "foo",
			templateVersion: "v1.0.0",
			expectedError: &errors.StatusError{
				ErrStatus: v1.Status{
					Status:  v1.StatusFailure,
					Code:    http.StatusNotFound,
					Reason:  v1.StatusReasonNotFound,
					Message: "NotFound",
				},
			},
			expectedStatusCode: http.StatusNotFound,
			expectedErrMessage: "not found",
		},
		{
			name:            "500 Internal Server Error",
			template:        v1alpha1.ClusterTemplate{},
			templateName:    "foo",
			templateVersion: "v1.0.0",
			expectedError: &errors.StatusError{
				ErrStatus: v1.Status{
					Status:  v1.StatusFailure,
					Code:    http.StatusInternalServerError,
					Reason:  v1.StatusReasonInternalError,
					Message: "Internal Server Error",
				},
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrMessage: "Internal Server Error",
		},
		{
			name: "500 Invalid ClusterConfiguration in ClusterTemplate",
			template: v1alpha1.ClusterTemplate{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-template-v0.0.1",
					Namespace: expectedActiveProjectID,
				},
				Spec: v1alpha1.ClusterTemplateSpec{
					ControlPlaneProviderType: "kubeadm",
					InfraProviderType:        "docker",
					KubernetesVersion:        "1.21.0",
					ClusterConfiguration:     "{invalid json}",
				},
			},
			templateName:       "test-template",
			templateVersion:    "v0.0.1",
			expectedError:      nil,
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrMessage: "invalid",
		},
	}

	for _, tt := range tests {
		server := createMockServerTemplateNameVersion(t, tt.template, expectedActiveProjectID, tt.expectedError)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", fmt.Sprintf("/v2/templates/%s/%s", tt.templateName, tt.templateVersion), nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		require.Equal(t, tt.expectedStatusCode, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, tt.expectedStatusCode)

		// Check the response status
		resp, err := api.ParseGetV2TemplatesNameVersionResponse(rr.Result())
		require.NoError(t, err, "api.ParseGetV2TemplatesNameVersionResponse() error = %v, want nil", err)

		var actualMessage *string
		var actualTemplateInfo *api.TemplateInfo
		switch rr.Code {
		case http.StatusOK:
			actualTemplateInfo = resp.JSON200
		case http.StatusNotFound:
			actualMessage = resp.JSON404.Message
		case http.StatusInternalServerError:
			actualMessage = resp.JSON500.Message
		}

		if tt.expectedErrMessage == "" {
			require.Equal(t, tt.expectedTemplateInfo, *actualTemplateInfo, "TemplateInfo = %v, want %v", *actualTemplateInfo, tt.expectedTemplateInfo)
		} else {
			require.Contains(t, *actualMessage, tt.expectedErrMessage, "ServeHTTP() body = %v, want %v", *actualMessage, tt.expectedErrMessage)
		}
	}
}

func createGetV2TemplatesNameStubServer(t *testing.T) *Server {
	template := v1alpha1.ClusterTemplate{}
	unstructuredTemplate, err := convert.ToUnstructured(template)
	require.NoError(t, err, "convertAnyToUnstructured() error = %v, want nil")
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(unstructuredTemplate, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzGetV2TemplatesName(f *testing.F) {
	f.Add("abc", "def",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name, ver string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2TemplatesNameStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.GetV2TemplatesNameVersionRequestObject{
			Name:    name,
			Version: ver,
			Params: api.GetV2TemplatesNameVersionParams{
				Activeprojectid: activeprojectid,
			},
		}
		_, _ = server.GetV2TemplatesNameVersion(context.Background(), req)
	})
}
