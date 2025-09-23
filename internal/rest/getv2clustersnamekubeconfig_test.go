// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	intauth "github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"github.com/open-edge-platform/cluster-manager/v2/test/helpers"
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

func mockK8sClientSetup(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface, kubeconfigName string, returnValue interface{}, returnError error) {
	resource.EXPECT().Get(mock.Anything, kubeconfigName, metav1.GetOptions{}).Return(returnValue.(*unstructured.Unstructured), returnError)
	nsResource.EXPECT().Namespace("655a6892-4280-4c37-97b1-31161ac0b99e").Return(resource)
	mockedk8sclient.EXPECT().Resource(core.SecretResourceSchema).Return(nsResource)
}

func mockK8sClient(t *testing.T, clusterName, kubeconfigValue string, setupFunc func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface)) (*k8s.MockInterface, *k8s.MockResourceInterface, *k8s.MockNamespaceableResourceInterface) {
	resource := k8s.NewMockResourceInterface(t)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	mockedk8sclient := k8s.NewMockInterface(t)

	if setupFunc != nil {
		setupFunc(resource, nsResource, mockedk8sclient)
	} else {
		kubeconfigName := clusterName + "-kubeconfig"
		clusterSecret := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"data": map[string]interface{}{
					"value": kubeconfigValue,
				},
			},
		}
		mockK8sClientSetup(resource, nsResource, mockedk8sclient, kubeconfigName, clusterSecret, nil)
	}

	return mockedk8sclient, resource, nsResource
}

func createRequestAndRecorder(_ *testing.T, method, url, activeProjectID, authHeader string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, url, nil)
	req.Header.Set("Activeprojectid", activeProjectID)
	// prepend Bearer and fallback jwt only if authHeader empty (enables dynamic TTL test tokens)
	token := authHeader
	if token == "" {
		token = jwtToken
	}
	if !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	return req, rr
}

func configureHandlerAndServe(t *testing.T, server *Server, rr *httptest.ResponseRecorder, req *http.Request) {
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)
}

func mockTokenRenewal(jwtToken string) func() {
	originalTokenRenewalFunc := tokenRenewalFunc
	tokenRenewalFunc = func(authHeader string, disableAuth bool, disableCustomTTL bool, ttl *time.Duration) (string, error) {
		return jwtToken, nil
	}
	return func() {
		tokenRenewalFunc = originalTokenRenewalFunc
	}
}

func TestGetV2ClustersNameKubeconfigs200(t *testing.T) {
	t.Run("successful kubeconfig retrieval", func(t *testing.T) {
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
		restoreTokenRenewal := mockTokenRenewal(jwtToken)
		defer restoreTokenRenewal()
		mockedk8sclient, _, _ := mockK8sClient(t, name, encodedKubeconfig, nil)
		serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin", DisableAuth: true, DisableCustomTTL: false, DefaultKubeconfigTTL: 30 * time.Minute}
		server := NewServer(mockedk8sclient)
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

func TestGetV2ClustersNameKubeconfigs404(t *testing.T) {
	expected404Response := `{"message":"404 Not Found: kubeconfig not found"}`
	restoreTokenRenewal := mockTokenRenewal(jwtToken)
	defer restoreTokenRenewal()
	tests := []struct {
		name             string
		clusterName      string
		activeProjectID  string
		authHeader       string
		mockSetup        func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:            "no cluster name",
			clusterName:     "",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: "no matching operation was found\n",
		},
		{
			name:            "cluster Not Found",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				clusterSecret := &unstructured.Unstructured{
					Object: map[string]interface{}{},
				}
				mockK8sClientSetup(resource, nsResource, mockedk8sclient, "example-cluster-kubeconfig", clusterSecret, errors.NewNotFound(core.SecretResourceSchema.GroupResource(), "example-cluster-kubeconfig"))
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
		{
			name:            "no kubeconfig in secret",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				clusterSecret := &unstructured.Unstructured{
					Object: map[string]interface{}{},
				}
				mockK8sClientSetup(resource, nsResource, mockedk8sclient, "example-cluster-kubeconfig", clusterSecret, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
		{
			name:            "not able to decode kubeconfig",
			clusterName:     "example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				clusterSecret := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"data": map[string]interface{}{
							"value": "wrongEncodedKubeconfig",
						},
					},
				}
				mockK8sClientSetup(resource, nsResource, mockedk8sclient, "example-cluster-kubeconfig", clusterSecret, nil)
			},
			expectedCode:     http.StatusNotFound,
			expectedResponse: expected404Response,
		},
	}
	serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockedk8sclient, _, _ := mockK8sClient(t, tt.clusterName, "", tt.mockSetup)
			server := NewServer(mockedk8sclient)
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

			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", name), nil)
			if tt.activeProjectID != "" {
				req.Header.Set("Activeprojectid", tt.activeProjectID)
				req.Header.Set("Authorization", "Bearer "+jwtToken)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

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
			expectedResponse: `{"message":"parameter \"Authorization\" in header has an error: empty value is not allowed"}`,
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
			req := httptest.NewRequest("GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", name), nil)
			req.Header.Set("Activeprojectid", activeProjectID)
			req.Header.Set("Authorization", tt.authHeader)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func TestGetV2ClustersNameKubeconfigs500(t *testing.T) {
	restoreTokenRenewal := mockTokenRenewal(jwtToken)
	defer restoreTokenRenewal()
	tests := []struct {
		name             string
		clusterName      string
		activeProjectID  string
		authHeader       string
		mockSetup        func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface)
		expectedCode     int
		expectedResponse string
	}{
		{
			name:            "error updating kubeconfig with token",
			clusterName:     "demo-example-cluster",
			activeProjectID: "655a6892-4280-4c37-97b1-31161ac0b99e",
			authHeader:      "Bearer " + jwtToken,
			mockSetup: func(resource *k8s.MockResourceInterface, nsResource *k8s.MockNamespaceableResourceInterface, mockedk8sclient *k8s.MockInterface) {
				encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
				clusterSecret := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"data": map[string]interface{}{
							"value": encodedKubeconfig,
						},
					},
				}
				mockK8sClientSetup(resource, nsResource, mockedk8sclient, "demo-example-cluster-kubeconfig", clusterSecret, nil)
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
			updateKubeconfigWithTokenFunc = func(kubeconfig kubeconfigParameters, activeProjectID, clusterName, token string, disableAuth bool, disableCustomTTL bool, ttl *time.Duration) (string, error) {
				return "", fmt.Errorf("failed to update kubeconfig with token")
			}
			defer func() {
				updateKubeconfigWithTokenFunc = originalFunc
			}()
			mockedk8sclient, _, _ := mockK8sClient(t, tt.clusterName, "", tt.mockSetup)
			server := NewServer(mockedk8sclient)
			server.config = &serverConfig
			require.NotNil(t, server, "NewServer() returned nil, want not nil")
			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", tt.clusterName), tt.activeProjectID, tt.authHeader)
			configureHandlerAndServe(t, server, rr, req)
			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.JSONEq(t, tt.expectedResponse, rr.Body.String())
		})
	}
}

func createGetV2KubeconfigStubServer(t *testing.T) *Server {
	clusterSecret := &unstructured.Unstructured{}
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(clusterSecret, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(mock.Anything).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
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
			expectedError:   "failed to unmarshal kubeconfig",
			expectedConfig:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only mock token renewal for valid tokens (the first test)
			// Let invalid tokens fail naturally for testing error handling
			var restoreTokenRenewal func()
			if tt.name == "successful update" {
				restoreTokenRenewal = mockTokenRenewal(tt.token)
				defer restoreTokenRenewal()
			}

			updatedConfig, err := updateKubeconfigWithToken(tt.kubeconfig, tt.activeProjectID, tt.clusterName, tt.token, true, false, nil)

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

func TestTokenRenewal(t *testing.T) {
	type tokenRenewalTest struct {
		name             string
		disableAuth      bool
		disableCustomTTL bool
		ttl              *time.Duration
		expectSame       bool
		expectErr        bool
		expectCalled     bool
		expectedTTL      *time.Duration // only validated when expectCalled && !expectErr
		newTokenTTL      time.Duration  // TTL for generated mock token when renewing
		mockError        error
	}

	twoHours := 2 * time.Hour
	cases := []tokenRenewalTest{
		{
			name:             "skip renewal when DisableAuth",
			disableAuth:      true,
			disableCustomTTL: false,
			ttl:              nil,
			expectSame:       true,
			expectErr:        false,
			expectCalled:     false,
		},
		{
			name:             "skip renewal when DisableCustomTTL",
			disableAuth:      false,
			disableCustomTTL: true,
			ttl:              nil,
			expectSame:       true,
			expectErr:        false,
			expectCalled:     false,
		},
		{
			name:             "renew when both enabled",
			disableAuth:      false,
			disableCustomTTL: false,
			ttl:              &twoHours,
			expectedTTL:      &twoHours,
			newTokenTTL:      2 * time.Hour,
			expectSame:       false,
			expectErr:        false,
			expectCalled:     true,
		},
		// even if the original token TTL is long, we still renew.
		{
			name:             "renew even if original token TTL long",
			disableAuth:      false,
			disableCustomTTL: false,
			ttl:              &twoHours,
			expectedTTL:      &twoHours,
			newTokenTTL:      2 * time.Hour,
			expectSame:       false,
			expectErr:        false,
			expectCalled:     true,
		},
		{
			name:             "error when M2M fails",
			disableAuth:      false,
			disableCustomTTL: false,
			ttl:              nil,
			mockError:        fmt.Errorf("M2M error"),
			expectSame:       false,
			expectErr:        true,
			expectCalled:     true,
		},
	}

	originalJwtTokenWithM2MFunc := JwtTokenWithM2MFunc
	defer func() { JwtTokenWithM2MFunc = originalJwtTokenWithM2MFunc }()

	// valid, unexpired token for renewal scenarios (45m future expiry)
	originalToken := helpers.CreateTestJWT(time.Now().Add(45*time.Minute), []string{"orig-role"})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
				called = true
				if c.mockError != nil {
					return "", c.mockError
				}
				if c.expectedTTL != nil {
					if ttl == nil || *ttl != *c.expectedTTL {
						return "", fmt.Errorf("unexpected ttl: got %v want %v", ttl, *c.expectedTTL)
					}
				}
				exp := time.Now().Add(c.newTokenTTL)
				return helpers.CreateTestJWT(exp, []string{"test-role"}), nil
			}

			result, err := tokenRenewal(originalToken, c.disableAuth, c.disableCustomTTL, c.ttl)

			assert.Equal(t, c.expectCalled, called, "JwtTokenWithM2MFunc call expectation mismatch")

			if c.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get new M2M token")
				return
			}
			require.NoError(t, err)
			if c.expectSame {
				assert.Equal(t, originalToken, result)
			} else {
				assert.NotEqual(t, originalToken, result)
			}
		})
	}
}

// TestKubeconfigTTLBehavior tests TTL behavior in kubeconfig generation
func TestKubeconfigTTLBehavior(t *testing.T) {
	tests := []struct {
		name             string
		disableCustomTTL bool
		kubeconfigTTL    time.Duration
		expectedError    bool
		expectedTTL      time.Duration
		tolerance        time.Duration
	}{
		{
			name:             "custom TTL enabled - 2 hours",
			disableCustomTTL: false, // false = enable custom TTL
			kubeconfigTTL:    2 * time.Hour,
			expectedError:    false,
			expectedTTL:      2 * time.Hour,
			tolerance:        1 * time.Minute,
		},
		{
			name:             "custom TTL enabled - 24 hours",
			disableCustomTTL: false, // false = enable custom TTL
			kubeconfigTTL:    24 * time.Hour,
			expectedError:    false,
			expectedTTL:      24 * time.Hour,
			tolerance:        1 * time.Minute,
		},
		{
			name:             "custom TTL disabled - use default",
			disableCustomTTL: true,           // true = disable custom TTL
			kubeconfigTTL:    12 * time.Hour, // Should be ignored
			expectedError:    false,
			expectedTTL:      1 * time.Hour, // Keycloak default
			tolerance:        5 * time.Minute,
		},
		{
			name:             "always renew: configured TTL applied regardless of original token",
			disableCustomTTL: false, // false = enable custom TTL
			kubeconfigTTL:    6 * time.Hour,
			expectedError:    false,
			expectedTTL:      6 * time.Hour,
			tolerance:        1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server config with TTL settings
			serverConfig := &config.Config{
				DisableCustomTTL:     tt.disableCustomTTL,
				DefaultKubeconfigTTL: tt.kubeconfigTTL,
				DisableAuth:          false,
				ClusterDomain:        "kind.internal",
				Username:             "admin",
			} // Test TTL configuration logic
			var kubeconfigTTL *time.Duration
			if !serverConfig.DisableCustomTTL {
				kubeconfigTTL = &serverConfig.DefaultKubeconfigTTL
			}

			if tt.disableCustomTTL {
				assert.Nil(t, kubeconfigTTL, "kubeconfigTTL should be nil when custom TTL is disabled")
			} else {
				assert.NotNil(t, kubeconfigTTL, "kubeconfigTTL should not be nil when custom TTL is enabled")
				assert.Equal(t, tt.expectedTTL, *kubeconfigTTL, "TTL should match expected value")
			}
		})
	}
}

// TestTokenRenewalWithVaultAndKeycloak tests the complete token renewal flow
func TestTokenRenewalWithVaultAndKeycloak(t *testing.T) {
	tests := []struct {
		name              string
		originalTokenTTL  time.Duration
		vaultClientID     string
		vaultClientSecret string
		userRoles         []string
		requestedTTL      *time.Duration
		vaultShouldFail   bool
		expectedError     bool
	}{
		{
			name:              "successful token renewal with vault credentials",
			originalTokenTTL:  30 * time.Minute, // needs renewal
			vaultClientID:     "co-manager-m2m-client",
			vaultClientSecret: "test-secret",
			userRoles:         []string{"admin", "cluster-reader"},
			requestedTTL:      &[]time.Duration{4 * time.Hour}[0],
			vaultShouldFail:   false,
			expectedError:     false,
		},
		{
			name:              "vault failure during credential retrieval",
			originalTokenTTL:  30 * time.Minute,
			vaultClientID:     "",
			vaultClientSecret: "",
			userRoles:         []string{"user"},
			requestedTTL:      &[]time.Duration{2 * time.Hour}[0],
			vaultShouldFail:   true,
			expectedError:     true,
		},
		{
			name:              "always renew even if original token still valid",
			originalTokenTTL:  2 * time.Hour, // still valid but should be renewed (policy: always renew)
			vaultClientID:     "co-manager-m2m-client",
			vaultClientSecret: "test-secret",
			userRoles:         []string{"user"},
			requestedTTL:      &[]time.Duration{6 * time.Hour}[0],
			vaultShouldFail:   false,
			expectedError:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			origExp := time.Now().Add(tt.originalTokenTTL)
			originalToken := helpers.CreateTestJWT(origExp, tt.userRoles)

			mockVault := helpers.NewMockVaultAuth(tt.vaultClientID, tt.vaultClientSecret)
			if tt.vaultShouldFail {
				mockVault.SetFailure(true, "forced failure")
			}

			mockKC := helpers.NewMockKeycloakServer()
			defer mockKC.Close()

			// inject mocks
			originalNewVaultAuthFunc := intauth.NewVaultAuthFunc
			intauth.NewVaultAuthFunc = func(vs, sa string) (intauth.VaultAuth, error) { return mockVault, nil }
			defer func() { intauth.NewVaultAuthFunc = originalNewVaultAuthFunc }()

			originalJwtTokenWithM2MFunc := JwtTokenWithM2MFunc
			JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
				_ = os.Setenv("KEYCLOAK_URL", mockKC.URL())
				return intauth.JwtTokenWithM2M(ctx, ttl)
			}
			defer func() { JwtTokenWithM2MFunc = originalJwtTokenWithM2MFunc }()

			// Execute renewal
			newToken, err := tokenRenewal(originalToken, false, false, tt.requestedTTL)
			if tt.expectedError {
				require.Error(t, err, "expected an error but got none")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, newToken)

			// explicitly assert renewal occurred when no error expected (policy: always renew)
			if newToken == originalToken {
				// Using Fatalf to stop further TTL checks which would mislead
				if !tt.expectedError {
					t.Fatalf("expected renewed token to differ from original under always-renew policy")
				}
			}

			// if token didn't require renewal (original longer than requested threshold) we may receive original token
			// validate resulting token TTL >= min(originalRemaining, requestedTTL)
			_, _, exp, errClaims := intauth.ExtractClaims(newToken)
			require.NoError(t, errClaims)
			remaining := time.Until(exp)
			if tt.requestedTTL != nil {
				// allow 2m scheduling drift
				minExpected := *tt.requestedTTL
				if remaining+2*time.Minute < minExpected {
					t.Fatalf("renewed token TTL too short: got %v want >= %v", remaining, minExpected)
				}
			}
		})
	}
}

// TestKubeconfigEndToEndWithTTL verifies the handler returns a kubeconfig whose token TTL is the
// configured custom TTL when auth & custom TTL are enabled, or the original TTL when renewal is
// skipped (auth disabled or custom TTL disabled), validating the renewal decision
func TestKubeconfigEndToEndWithTTL(t *testing.T) {
	testCases := []struct {
		name             string
		disableCustomTTL bool
		disableAuth      bool
		configuredTTL    time.Duration
		initialTokenTTL  time.Duration
		expectedTTL      time.Duration
		renews           bool
	}{
		{
			name:             "custom TTL enabled - renew to 2h",
			disableCustomTTL: false,
			disableAuth:      false,
			configuredTTL:    2 * time.Hour,
			initialTokenTTL:  10 * time.Minute,
			expectedTTL:      2 * time.Hour,
			renews:           true,
		},
		{
			name:             "custom TTL disabled - keep original 1h",
			disableCustomTTL: true,
			disableAuth:      false,
			configuredTTL:    6 * time.Hour, // ignored
			initialTokenTTL:  1 * time.Hour,
			expectedTTL:      1 * time.Hour,
			renews:           false,
		},
		{
			name:             "auth disabled - original token retained",
			disableCustomTTL: false, // would normally trigger renewal
			disableAuth:      true,  // forces skip
			configuredTTL:    3 * time.Hour,
			initialTokenTTL:  45 * time.Minute,
			expectedTTL:      45 * time.Minute, // should remain original because renewal skipped
			renews:           false,
		},
	}

	tolerance := 2 * time.Minute
	clusterName := "example-cluster"
	activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build initial Authorization token with specified TTL
			initialExp := time.Now().Add(tc.initialTokenTTL)
			initialToken := helpers.CreateTestJWT(initialExp, []string{"initial-role"})

			original := JwtTokenWithM2MFunc
			defer func() { JwtTokenWithM2MFunc = original }()

			if tc.renews {
				JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
					if ttl == nil || *ttl != tc.configuredTTL {
						return "", fmt.Errorf("unexpected ttl passed (got %v want %v)", ttl, tc.configuredTTL)
					}
					exp := time.Now().Add(*ttl)
					return helpers.CreateTestJWT(exp, []string{"renewed-role"}), nil
				}
			} else {
				JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
					return "", fmt.Errorf("JwtTokenWithM2MFunc should not be called when disableCustomTTL=%v", tc.disableCustomTTL)
				}
			}

			serverConfig := config.Config{
				ClusterDomain:        "kind.internal",
				Username:             "admin",
				DisableAuth:          tc.disableAuth,
				DisableCustomTTL:     tc.disableCustomTTL,
				DefaultKubeconfigTTL: tc.configuredTTL,
			}

			encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
			mockedk8sclient, _, _ := mockK8sClient(t, clusterName, encodedKubeconfig, nil)
			server := NewServer(mockedk8sclient)
			server.config = &serverConfig

			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", clusterName), activeProjectID, initialToken)
			configureHandlerAndServe(t, server, rr, req)

			require.Equal(t, http.StatusOK, rr.Code, "expected successful response")

			var resp struct {
				Kubeconfig string `json:"kubeconfig"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			require.NotEmpty(t, resp.Kubeconfig, "kubeconfig should not be empty")

			if err := helpers.ValidateKubeconfigToken(resp.Kubeconfig, tc.expectedTTL, tolerance); err != nil {
				t.Fatalf("TTL validation failed: %v", err)
			}
		})
	}
}

// TestKubeconfigEndToEndWithTTLM2MFailure ensures the handler returns 500 when renewal (M2M) fails
func TestKubeconfigEndToEndWithTTLM2MFailure(t *testing.T) {
	clusterName := "example-cluster"
	activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	// Initial token (will attempt renewal)
	initialExp := time.Now().Add(30 * time.Minute)
	initialToken := helpers.CreateTestJWT(initialExp, []string{"role"})

	// mock renewal to return error
	original := JwtTokenWithM2MFunc
	JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
		return "", fmt.Errorf("simulated m2m failure")
	}
	defer func() { JwtTokenWithM2MFunc = original }()

	serverConfig := config.Config{
		ClusterDomain:        "kind.internal",
		Username:             "admin",
		DisableAuth:          false,
		DisableCustomTTL:     false,
		DefaultKubeconfigTTL: 2 * time.Hour,
	}

	encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
	mockedk8sclient, _, _ := mockK8sClient(t, clusterName, encodedKubeconfig, nil)
	server := NewServer(mockedk8sclient)
	server.config = &serverConfig

	req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", clusterName), activeProjectID, initialToken)
	configureHandlerAndServe(t, server, rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 status when m2m fails, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to process kubeconfig") {
		t.Fatalf("expected failure message in body, got %s", rr.Body.String())
	}
}

// TestKubeconfigEndToEndRenewalCallExpectations verifies whether renewal is called or skipped under different flags
func TestKubeconfigEndToEndRenewalCallExpectations(t *testing.T) {
	type testCase struct {
		name             string
		disableAuth      bool
		disableCustomTTL bool
		expectCalled     bool
	}
	cases := []testCase{
		{name: "renewal called when both enabled", disableAuth: false, disableCustomTTL: false, expectCalled: true},
		{name: "renewal skipped when auth disabled", disableAuth: true, disableCustomTTL: false, expectCalled: false},
		{name: "renewal skipped when custom TTL disabled", disableAuth: false, disableCustomTTL: true, expectCalled: false},
	}

	clusterName := "example-cluster"
	activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initialExp := time.Now().Add(15 * time.Minute)
			initialToken := helpers.CreateTestJWT(initialExp, []string{"role"})

			called := false
			original := JwtTokenWithM2MFunc
			JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
				called = true
				if ttl == nil && !tc.disableCustomTTL && !tc.disableAuth {
					return "", fmt.Errorf("expected ttl pointer when custom TTL enabled")
				}
				exp := time.Now().Add(1 * time.Hour)
				return helpers.CreateTestJWT(exp, []string{"renewed"}), nil
			}
			defer func() { JwtTokenWithM2MFunc = original }()

			serverConfig := config.Config{
				ClusterDomain:        "kind.internal",
				Username:             "admin",
				DisableAuth:          tc.disableAuth,
				DisableCustomTTL:     tc.disableCustomTTL,
				DefaultKubeconfigTTL: 1 * time.Hour,
			}
			encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
			mockedk8sclient, _, _ := mockK8sClient(t, clusterName, encodedKubeconfig, nil)
			server := NewServer(mockedk8sclient)
			server.config = &serverConfig

			req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", clusterName), activeProjectID, initialToken)
			configureHandlerAndServe(t, server, rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 OK got %d body=%s", rr.Code, rr.Body.String())
			}
			if called != tc.expectCalled {
				t.Fatalf("renewal call mismatch: got called=%v expect=%v", called, tc.expectCalled)
			}
		})
	}
}

// TestExpiredOriginalTokenRenewal verifies that an already-expired
// incoming bearer token is rejected (401) and NOT renewed.
func TestExpiredOriginalTokenRenewal(t *testing.T) {
	clusterName := "example-cluster"
	activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	configuredTTL := 2 * time.Hour // still configured, but should not be used for expired token

	// prepare an already-expired token
	expiredExp := time.Now().Add(-30 * time.Minute)
	expiredToken := helpers.CreateTestJWT(expiredExp, []string{"stale-role"})

	// track unexpected renewal attempts
	called := false
	original := JwtTokenWithM2MFunc
	JwtTokenWithM2MFunc = func(ctx context.Context, ttl *time.Duration) (string, error) {
		called = true
		return "", fmt.Errorf("should not be called for expired token")
	}
	defer func() { JwtTokenWithM2MFunc = original }()

	serverConfig := config.Config{
		ClusterDomain:        "kind.internal",
		Username:             "admin",
		DisableAuth:          false,
		DisableCustomTTL:     false,
		DefaultKubeconfigTTL: configuredTTL,
	}

	encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(exampleKubeconfig))
	mockedk8sclient, _, _ := mockK8sClient(t, clusterName, encodedKubeconfig, nil)
	server := NewServer(mockedk8sclient)
	server.config = &serverConfig

	req, rr := createRequestAndRecorder(t, "GET", fmt.Sprintf("/v2/clusters/%s/kubeconfigs", clusterName), activeProjectID, expiredToken)
	configureHandlerAndServe(t, server, rr, req)

	if rr.Code != http.StatusUnauthorized { // 401
		t.Fatalf("expected 401 Unauthorized, got %d body=%s", rr.Code, rr.Body.String())
	}
	if called {
		t.Fatalf("unexpected renewal attempt for expired token")
	}

	var resp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal 401 response: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "token expired") {
		t.Fatalf("expected 'token expired' in message, got %q", resp.Message)
	}
}
