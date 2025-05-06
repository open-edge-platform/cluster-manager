// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestDeleteV2TemplatesNameVersion(t *testing.T) {
	expectedActiveProjectID := "28f35814-dd5d-11ef-93a6-17b771bdd27a"
	tests := []struct {
		name                 string
		expectedStatusCode   int
		expectedErrorMessage string
		mockDeleteReturn     error
	}{
		{
			name:               "204 No Content",
			expectedStatusCode: http.StatusNoContent,
			mockDeleteReturn:   nil,
		},
		{
			name:                 "400 Bad Request",
			expectedStatusCode:   http.StatusBadRequest,
			expectedErrorMessage: "Bad request",
			mockDeleteReturn: &errors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Code:    http.StatusBadRequest,
					Reason:  metav1.StatusReasonBadRequest,
					Message: "Bad request",
				},
			},
		},
		{
			name:                 "404 Not Found",
			expectedStatusCode:   http.StatusNotFound,
			expectedErrorMessage: "Is not found",
			mockDeleteReturn: &errors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Code:    http.StatusNotFound,
					Reason:  metav1.StatusReasonNotFound,
					Message: "Is not found",
				},
			},
		},
		{
			name:                 "409 Conflict - template in use",
			expectedStatusCode:   http.StatusConflict,
			expectedErrorMessage: "Is in use",
			mockDeleteReturn: &errors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Code:    http.StatusConflict,
					Reason:  metav1.StatusReasonConflict,
					Message: "Is in use",
				},
			},
		},
		{
			name:                 "500 Internal Server Error",
			expectedStatusCode:   http.StatusInternalServerError,
			expectedErrorMessage: "Internal server error",
			mockDeleteReturn: &errors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Code:    http.StatusInternalServerError,
					Reason:  metav1.StatusReasonInternalError,
					Message: "Internal server error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := k8s.NewMockResourceInterface(t)
			resource.EXPECT().Delete(mock.Anything, mock.Anything, metav1.DeleteOptions{}).Return(tt.mockDeleteReturn)
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
			req := httptest.NewRequest("DELETE", "/v2/templates/fakename/v1.0.0", nil)
			req.Header.Set("Activeprojectid", expectedActiveProjectID)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// serve the request
			handler.ServeHTTP(rr, req)

			// check the response status
			require.Equal(t, tt.expectedStatusCode, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, tt.expectedStatusCode)

			// check the response body
			if tt.expectedErrorMessage != "" {
				resp, err := api.ParseDeleteV2TemplatesNameVersionResponse(rr.Result())
				require.NoError(t, err, "json.ParseDeleteV2TemplatesNameVersionResponse() error = %v, want nil", err)

				var actualMessage *string
				switch rr.Code {
				case http.StatusBadRequest:
					actualMessage = resp.JSON400.Message
				case http.StatusNotFound:
					actualMessage = resp.JSON404.Message
				case http.StatusConflict:
					actualMessage = resp.JSON409.Message
				case http.StatusInternalServerError:
					actualMessage = resp.JSON500.Message
				}

				require.Contains(t, *actualMessage, tt.expectedErrorMessage, "ServeHTTP() body = %v, want %v", *actualMessage, tt.expectedErrorMessage)
			}
		})
	}
}

func createDeleteV2TemplatesNameStubServer(t *testing.T) *Server {
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Delete(mock.Anything, mock.Anything, metav1.DeleteOptions{}).Return(nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzDeleteV2TemplatesName(f *testing.F) {
	f.Add("abc", "def",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name, ver string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createDeleteV2TemplatesNameStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.DeleteV2TemplatesNameVersionRequestObject{
			Name:    name,
			Version: ver,
			Params: api.DeleteV2TemplatesNameVersionParams{
				Activeprojectid: activeprojectid,
			},
		}
		_, _ = server.DeleteV2TemplatesNameVersion(context.Background(), req)
	})
}
