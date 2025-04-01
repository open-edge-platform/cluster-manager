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
	"github.com/open-edge-platform/cluster-manager/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetV2TemplatesNameVersions(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	tests := []struct {
		name               string
		templates          []v1alpha1.ClusterTemplate
		expectedError      error
		expectedStatusCode int
		expectedResponse   api.VersionList
		expectedErrMessage string
	}{
		{
			name:               "200 OK",
			templates:          []v1alpha1.ClusterTemplate{template1, template2},
			expectedError:      nil,
			expectedStatusCode: http.StatusOK,
			expectedResponse:   api.VersionList{VersionList: &[]string{"v0.0.1", "v0.0.2"}},
		},
		{
			name:               "404 Not Found",
			templates:          []v1alpha1.ClusterTemplate{},
			expectedError:      nil,
			expectedStatusCode: http.StatusNotFound,
			expectedErrMessage: "not found",
		},
		{
			name:      "500 Internal Server Error",
			templates: nil,
			expectedError: &errors.StatusError{
				ErrStatus: v1.Status{
					Status:  v1.StatusFailure,
					Code:    http.StatusInternalServerError,
					Reason:  v1.StatusReasonInternalError,
					Message: "Internal server error",
				},
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrMessage: "Internal server error",
		},
		{
			name: "500 Invalid name in ClusterTemplate",
			templates: []v1alpha1.ClusterTemplate{
				{
					ObjectMeta: v1.ObjectMeta{Name: "test-template-NO-VERSION", Namespace: "6ef4177e-dcc5-11ef-8262-7332735f750a"},
					Spec: v1alpha1.ClusterTemplateSpec{
						ControlPlaneProviderType: "kubeadm",
						InfraProviderType:        "docker",
						KubernetesVersion:        "1.21.0",
					},
				},
			},
			expectedError: &errors.StatusError{
				ErrStatus: v1.Status{
					Status:  v1.StatusFailure,
					Code:    http.StatusInternalServerError,
					Reason:  v1.StatusReasonInternalError,
					Message: "invalid clusterTemplate name format",
				},
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrMessage: "invalid clusterTemplate name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServerTemplates(t, tt.templates, expectedActiveProjectID, tt.expectedError)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			handler, err := server.ConfigureHandler()
			require.Nil(t, err)

			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/templates/%s/versions", templateInfo1.Name), nil)
			req.Header.Set("Activeprojectid", expectedActiveProjectID)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedStatusCode, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, tt.expectedStatusCode)

			// check the response body
			if tt.expectedErrMessage != "" {
				resp, err := api.ParseGetV2TemplatesNameVersionsResponse(rr.Result())
				require.NoError(t, err, "api.ParseGetV2TemplatesNameVersionsResponse() error = %v, want nil", err)

				var actualMessage *string
				switch rr.Code {
				case http.StatusBadRequest:
					actualMessage = resp.JSON400.Message
				case http.StatusNotFound:
					actualMessage = resp.JSON404.Message
				case http.StatusInternalServerError:
					actualMessage = resp.JSON500.Message
				}

				require.Contains(t, *actualMessage, tt.expectedErrMessage, "ServeHTTP() body = %v, want %v", *actualMessage, tt.expectedErrMessage)
			}
		})
	}
}

func FuzzGetV2VersionTemplatesName(f *testing.F) {
	f.Add("abc",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2TemplatesStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.GetV2TemplatesNameVersionsRequestObject{
			Name: name,
			Params: api.GetV2TemplatesNameVersionsParams{
				Activeprojectid: activeprojectid,
			},
		}
		_, _ = server.GetV2TemplatesNameVersions(context.Background(), req)
	})
}
