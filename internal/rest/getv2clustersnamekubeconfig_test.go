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

	"github.com/open-edge-platform/cluster-manager/internal/config"
	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/pkg/api"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	return req, rr
}

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

		mockedk8sclient, _, _ := mockK8sClient(t, name, encodedKubeconfig, nil)
		serverConfig := config.Config{ClusterDomain: "kind.internal", Username: "admin"}
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
			expectedResponse: `{"message":"failed getting kubeconfig for cluster example-cluster in namespace 655a6892-4280-4c37-97b1-31161ac0b99e"}`,
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
			expectedResponse: `{"message":"failed to get kubeconfig from secret: namespace=655a6892-4280-4c37-97b1-31161ac0b99e, name=example-cluster"}`,
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
			expectedResponse: `{"message":"failed to decode kubeconfig: namespace=655a6892-4280-4c37-97b1-31161ac0b99e, name=example-cluster"}`,
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
			expectedResponse: `{"message":"failed to update kubeconfig with token"}`,
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
