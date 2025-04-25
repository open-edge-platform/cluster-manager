// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var exampleKubeconfig = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: data
    server: http://edge-connect-gateway-cluster-connect-gateway.orch-cluster.svc:8080/kubernetes/655a6892-4280-4c37-97b1-31161ac0b99e-example-cluster
  name: example-cluster
contexts:
- context:
    cluster: example-cluster
    user: example-user
  name: example-context
current-context: example-cluster-admin@example-cluster
kind: Config
preferences: {}
users:
- name: example-user
  user:
    token: example-token`

var exampleKubeconfigWithToken = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: data
    server: https://connect-gateway.kind.internal:443/kubernetes/655a6892-4280-4c37-97b1-31161ac0b99e-example-cluster
  name: example-cluster
contexts:
- context:
    cluster: example-cluster
    user: example-cluster-admin
  name: example-cluster-admin@example-cluster
current-context: example-cluster-admin@example-cluster
kind: Config
preferences: {}
users:
- name: example-cluster-admin
  user:
    token: ` + jwtToken

var jwtToken = "eyJhbGciOiJQUzUxMiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICI2eE1yTjFSOVB3UGZnWFVaTFlTYW55ejRXa2hjS040WG8t" +
	"ZWxCRVdYaXM0In0.eyJleHAiOjE3NDA0ODgwNDQsImlhdCI6MTc0MDQ4NDQ0NCwianRpIjoiMmQxNDQ4NjctMjRkMS00OGQyL" +
	"WJhODUtMWEzNWZmOTQ2MmUwIiwiaXNzIjoiaHR0cHM6Ly9rZXljbG9hay5raW5kLmludGVybmFsL3JlYWxtcy9tYXN0ZXIiLCJ" +
	"zdWIiOiI5MTdiNDY4Ni1lZDJiLTRhNTYtODhjMy1mZTFkZTg5YTcwYjUiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJzeXN0ZW0tY" +
	"2xpZW50Iiwic2lkIjoiMTdhNmIxNzYtMjU2Ni00NTM2LTk3YTgtN2M3NDEyMGE3ZTUzIiwicmVhbG1fYWNjZXNzIjp7InJvbGV" +
	"zIjpbImNyZWF0ZS1yZWFsbSIsImRlZmF1bHQtcm9sZXMtbWFzdGVyIiwib2ZmbGluZV9hY2Nlc3MiLCJhZG1pbiIsInVtYV9hd" +
	"XRob3JpemF0aW9uIl19LCJyZXNvdXJjZV9hY2Nlc3MiOnsibWFzdGVyLXJlYWxtIjp7InJvbGVzIjpbInZpZXctcmVhbG0iLCJ" +
	"2aWV3LWlkZW50aXR5LXByb3ZpZGVycyIsIm1hbmFnZS1pZGVudGl0eS1wcm92aWRlcnMiLCJpbXBlcnNvbmF0aW9uIiwiY3JlY" +
	"XRlLWNsaWVudCIsIm1hbmFnZS11c2VycyIsInF1ZXJ5LXJlYWxtcyIsInZpZXctYXV0aG9yaXphdGlvbiIsInF1ZXJ5LWNsaWV" +
	"udHMiLCJxdWVyeS11c2VycyIsIm1hbmFnZS1ldmVudHMiLCJtYW5hZ2UtcmVhbG0iLCJ2aWV3LWV2ZW50cyIsInZpZXctdXNlc" +
	"nMiLCJ2aWV3LWNsaWVudHMiLCJtYW5hZ2UtYXV0aG9yaXphdGlvbiIsIm1hbmFnZS1jbGllbnRzIiwicXVlcnktZ3JvdXBzIl1" +
	"9LCJhY2NvdW50Ijp7InJvbGVzIjpbIm1hbmFnZS1hY2NvdW50IiwibWFuYWdlLWFjY291bnQtbGlua3MiLCJ2aWV3LXByb2Zpb" +
	"GUiXX19LCJzY29wZSI6Im9wZW5pZCBlbWFpbCByb2xlcyBwcm9maWxlIiwiZW1haWxfdmVyaWZpZWQiOmZhbHNlLCJwcmVmZXJ" +
	"yZWRfdXNlcm5hbWUiOiJhZG1pbiJ9.dvL2Qlcihnf1lhjwb-Fi_NjG-E394jNJUVPX-pGxTunBJLOnI05UeYyGKvxhWUM39a5S" +
	"8nXpeT-mGMK2jY4TqIQ7JeyL1yck9sQb7QU2zOlxBUtMK-fd9gYb0_SYsz7muEv3cMfs1-UIRfoKWPox3D5-PDXbb6oHbR8XJp" +
	"1I49KlWav_TjtVH1eLVwVZmXUCLDBptVFeiutLO21a7Cnb08wHPzZTddBkmBVcdgSmZF_Es0R2vmVNHR4TeNheHJX5lfCO5ufv" +
	"hpRMj0oMJzeK6TpFosmNpVtupw1KQkQlocCwMr9viNk-LCxOzHRdAWexsNNGclIJNBJGVT1GUWOFM50PzuLcB66xp8CHruOTSd" +
	"Ys2gVPnaJEjyGQk3o_ZpqxbUQB2uOe2GMKetxnSEokwYtmd6jsXoVF0qSzJX_rz1nqHSWQoynHXwl42HLyOR1XCQllFsvkqHGQ" +
	"_ZchiTpGi8PJv8WBcNp3Cu5tCQVCEFoYcNy6QPffYWpHi4MJvAKU-xNYmLrSzZlTsmj33eahRB7gAFrRMzFnU3MetjUvcFiZ3Z" +
	"hO7CiZFgpLpyWFTFN6kaWOTomdcsiDGQDeFLjV-P8v1206SMI3ywkcEfy-HaV3Cg7nFSpngwG0aBVKOgOi7vX-XZBmbPZb8cIC" +
	"CzfxjQ4QEDlUz2hxkgOYsVu7_0k"

func escapeNewlines(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

func mockK8sClient(t *testing.T, clusterName, kubeconfigValue string, namespace string, setupFunc func(mockK8sClient *k8s.MockClient)) *k8s.MockClient {
	mockK8sClient := k8s.NewMockClient(t)

	if setupFunc != nil {
		setupFunc(mockK8sClient)
	} else {
		kubeconfigName := clusterName + "-kubeconfig"
		clusterSecret := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"data": map[string]interface{}{
					"value": kubeconfigValue,
				},
			},
		}
		mockK8sClient.EXPECT().GetCached(mock.Anything, core.SecretResourceSchema, namespace, kubeconfigName).Return(clusterSecret, nil)
	}

	return mockK8sClient
}

// Helper functions

// createRequestAndRecorder creates a new request and response recorder for testing
func createRequestAndRecorder(t *testing.T, method, path, activeProjectID, authHeader string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)
	if activeProjectID != "" {
		req.Header.Set("Activeprojectid", activeProjectID)
	}
	if authHeader != "" {
		if strings.Contains(authHeader, "InvalidToken") {
			req.Header.Set("Authorization", authHeader)
		} else if strings.HasPrefix(authHeader, "Bearer ") {
			req.Header.Set("Authorization", authHeader)
		} else {
			req.Header.Set("Authorization", "Bearer "+authHeader)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	return req, httptest.NewRecorder()
}

// configureHandlerAndServe configures the handler and serves the request
func configureHandlerAndServe(t *testing.T, server *Server, rr *httptest.ResponseRecorder, req *http.Request) {
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)
}

func TestGetV2ClustersNameKubeconfigs200(t *testing.T) {
	t.Run("successful kubeconfig retrieval", func(t *testing.T) {
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))

		mockK8sClient := k8s.NewMockClient(t)
		mockK8sClient.EXPECT().GetCached(mock.Anything, core.SecretResourceSchema, activeProjectID, name+"-kubeconfig").Return(&unstructured.Unstructured{
			Object: map[string]interface{}{
				"data": map[string]interface{}{
					"value": encodedKubeconfig,
				},
			},
		}, nil)

		serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
		server := NewServer(mockK8sClient)
		server.config = &serverConfig
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", name), activeProjectID, jwtToken)

		// Create a handler with middleware to serve the request
		configureHandlerAndServe(t, server, rr, req)

		// Check the response
		assert.Equal(t, http.StatusOK, rr.Code)
		expectedResponse := fmt.Sprintf(`{"kubeconfig":"%s\n"}`, escapeNewlines(exampleKubeconfigWithToken))
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

// Update TestGetV2ClustersNameKubeconfigs404 to use the new mock approach
func TestGetV2ClustersNameKubeconfigs404(t *testing.T) {
	expected404Response := `{"message":"404 Not Found: kubeconfig not found"}`
	tests := []struct {
		name             string
		clusterName      string
		activeProjectID  string
		authHeader       string
		mockSetup        func(mockK8sClient *k8s.MockClient)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:             "no cluster name",
			clusterName:      "",
			activeProjectID:  "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:       "Bearer " + jwtToken, // Add valid token
			mockSetup:        nil,
			expectedCode:     http.StatusNotFound,
			expectedResponse: "no matching operation was found\n",
		},
		{
			name:            "cluster Not Found",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:      "Bearer " + jwtToken, // Add valid token
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				mockK8sClient.EXPECT().GetCached(
					mock.Anything,
					core.SecretResourceSchema,
					"655a6892-4280-4c37-97b1-31161ac0b99e",
					"example-cluster-kubeconfig",
				).Return(nil, errors.NewNotFound(core.SecretResourceSchema.GroupResource(), "example-cluster-kubeconfig"))
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
		{
			name:            "no kubeconfig in secret",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:      "Bearer " + jwtToken, // Add valid token
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				mockK8sClient.EXPECT().GetCached(
					mock.Anything,
					core.SecretResourceSchema,
					"655a6892-4280-4c37-97b1-31161ac0b99e",
					"example-cluster-kubeconfig",
				).Return(&unstructured.Unstructured{
					Object: map[string]interface{}{},
				}, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
		{
			name:            "not able to decode kubeconfig",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:      "Bearer " + jwtToken, // Add valid token
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				mockK8sClient.EXPECT().GetCached(
					mock.Anything,
					core.SecretResourceSchema,
					"655a6892-4280-4c37-97b1-31161ac0b99e",
					"example-cluster-kubeconfig",
				).Return(&unstructured.Unstructured{
					Object: map[string]interface{}{
						"data": map[string]interface{}{
							"value": "wrongEncodedKubeconfig",
						},
					},
				}, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
	}

	serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockK8sClient := k8s.NewMockClient(t)
			if tt.mockSetup != nil {
				tt.mockSetup(mockK8sClient)
			}
			server := NewServer(mockK8sClient)
			server.config = &serverConfig
			require.NotNil(t, server, "NewServer() returned nil, want not nil")
			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", tt.clusterName), tt.activeProjectID, tt.authHeader)

			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			if tt.name == "no cluster name" {
				assert.Equal(t, tt.expectedResponse, rr.Body.String())
			} else {
				assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
			}
		})
	}
}

func TestGetV2ClustersNameKubeconfig400(t *testing.T) {
	tests := []struct {
		name             string
		activeProjectID  string
		authHeader       string
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
			authHeader:       jwtToken,
			expectedCode:     http.StatusBadRequest,
			expectedResponse: `{"message":"no active project id provided"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "example-cluster"
			server := NewServer(nil)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", name), tt.activeProjectID, tt.authHeader)

			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func TestGetV2ClustersNameKubeconfigs401(t *testing.T) {
	tests := []struct {
		name             string
		authHeader       string
		expectedCode     int
		expectedResponse string
	}{
		{
			name:             "missing authorization header", // this is captured in the middleware
			authHeader:       "",
			expectedCode:     http.StatusBadRequest,
			expectedResponse: `{"message":"parameter \"Authorization\" in header has an error: value is required but missing"}`,
		},
		{
			name:             "invalid authorization header",
			authHeader:       "InvalidToken",
			expectedCode:     http.StatusUnauthorized,
			expectedResponse: `{"message":"Unauthorized: invalid Authorization header"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "example-cluster"
			activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
			server := NewServer(nil)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")
			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", name), activeProjectID, tt.authHeader)
			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func TestGetV2ClustersNameKubeconfigs500(t *testing.T) {
	tests := []struct {
		name             string
		clusterName      string
		activeProjectID  string
		authHeader       string
		mockSetup        func(mockK8sClient *k8s.MockClient)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:            "error updating kubeconfig with token",
			clusterName:     "demo-example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:      "Bearer " + jwtToken,
			mockSetup: func(mockK8sClient *k8s.MockClient) {
				encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
				mockK8sClient.EXPECT().GetCached(
					mock.Anything,
					core.SecretResourceSchema,
					"655a6892-4280-4c37-97b1-31161ac0b99e",
					"demo-example-cluster-kubeconfig",
				).Return(&unstructured.Unstructured{
					Object: map[string]interface{}{
						"data": map[string]interface{}{
							"value": encodedKubeconfig,
						},
					},
				}, nil)
			},
			expectedCode:     http.StatusInternalServerError,
			expectedResponse: `{"message":"500 Internal Server Error: failed to process kubeconfig"}`,
		},
	}
	serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// function variable with a mock implementation
			originalFunc := updateKubeconfigWithTokenFunc
			updateKubeconfigWithTokenFunc = func(kubeconfig kubeconfigParameters, activeProjectID, clusterName, token string) (string, error) {
				return "", fmt.Errorf("failed to update kubeconfig with token")
			}
			defer func() {
				updateKubeconfigWithTokenFunc = originalFunc
			}()

			// Create mock client
			mockK8sClient := k8s.NewMockClient(t)
			if tt.mockSetup != nil {
				tt.mockSetup(mockK8sClient)
			}

			server := NewServer(mockK8sClient)
			server.config = &serverConfig
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", tt.clusterName), tt.activeProjectID, tt.authHeader)
			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func FuzzGetV2Kubeconfig(f *testing.F) {
	f.Add("abc", byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name string, u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2KubeconfigStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.GetV2ClustersNameKubeconfigsParams{
			Activeprojectid: activeprojectid,
		}
		req := api.GetV2ClustersNameKubeconfigsRequestObject{
			Name:   name,
			Params: params,
		}
		_, _ = server.GetV2ClustersNameKubeconfigs(context.Background(), req)
	})
}

func TestUpdateKubeconfigWithToken(t *testing.T) {
	exampleKubeconfigWithoutJwtToken := exampleKubeconfig

	tests := []struct {
		name            string
		kubeconfig      kubeconfigParameters
		activeProjectID string
		clusterName     string
		token           string
		expectedError   string
		expectedConfig  string
	}{
		{
			name: "successful update",
			kubeconfig: kubeconfigParameters{
				serverCA:         "data",
				clusterDomain:    "kind.internal",
				userName:         "admin",
				kubeConfigDecode: exampleKubeconfigWithoutJwtToken,
			},
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			clusterName:     "example-cluster",
			token:           jwtToken,
			expectedError:   "",
			expectedConfig:  exampleKubeconfigWithToken,
		},
		{
			name: "failed to unmarshal kubeconfig",
			kubeconfig: kubeconfigParameters{
				serverCA:         "caData",
				clusterDomain:    "example.com",
				userName:         "kubernetes-admin",
				kubeConfigDecode: `invalid-kubeconfig`,
			},
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			clusterName:     "example-cluster",
			token:           "new-token",
			expectedError:   "failed to unmarshal kubeconfig: yaml: unmarshal errors:",
			expectedConfig:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedConfig, err := updateKubeconfigWithToken(tt.kubeconfig, tt.activeProjectID, tt.clusterName, tt.token)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.YAMLEq(t, tt.expectedConfig, updatedConfig)
			}
		})
	}
}

func createGetV2KubeconfigStubServer(t *testing.T) *Server {
	mockK8sClient := k8s.NewMockClient(t)

	// Mock the GetCached call that might be used to retrieve the kubeconfig
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"data": map[string]interface{}{
				"value": base64.StdEncoding.EncodeToString([]byte("dummy-kubeconfig")),
			},
		},
	}, nil).Maybe()

	serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
	server := NewServer(mockK8sClient)
	server.config = &serverConfig
	return server
}
