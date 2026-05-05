// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestProjectScopedClusterPathRewritesToExistingHandler(t *testing.T) {
	projectID := "12345678-1234-1234-1234-123456789012"
	projectName := "team-a"

	nexus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/projects/"+projectName, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"name": projectName,
			"status": map[string]any{
				"projectStatus": map[string]any{
					"uID": projectID,
				},
			},
		}))
	}))
	defer nexus.Close()

	server := createMockServer(t, []capi.Cluster{generateCluster(ptr("alpha"), ptr("v1.30.1"))}, projectID)
	WithConfig(&config.Config{ProjectServiceURL: nexus.URL})(server)

	handler, err := server.ConfigureHandler()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v2/projects/"+projectName+"/clusters", nil)
	req.Header.Set("Authorization", bearerTokenWithProjectRole(projectID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response api.GetV2Clusters200JSONResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.Equal(t, int32(1), response.TotalElements)
	require.Len(t, *response.Clusters, 1)
	require.Equal(t, "alpha", *(*response.Clusters)[0].Name)
}

func bearerTokenWithProjectRole(projectID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"realm_access": map[string]any{
			"roles": []string{projectID + "_member"},
		},
	})

	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		panic(err)
	}

	return "Bearer " + signed
}
