// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var JwtTokenWithM2MFunc = auth.JwtTokenWithM2M
var updateKubeconfigWithTokenFunc = updateKubeconfigWithToken
var tokenRenewalFunc = tokenRenewal

// (GET /v2/clusters/{name}/kubeconfigs)
func (s *Server) GetV2ClustersNameKubeconfigs(ctx context.Context, request api.GetV2ClustersNameKubeconfigsRequestObject) (api.GetV2ClustersNameKubeconfigsResponseObject, error) {
	namespace := request.Params.Activeprojectid.String()

	authHeader := request.Params.Authorization
	if !strings.HasPrefix(authHeader, auth.BearerPrefix) {
		slog.Error("invalid Authorization header", "authHeader", authHeader)
		return api.GetV2ClustersNameKubeconfigs401JSONResponse{
			N401UnauthorizedJSONResponse: api.N401UnauthorizedJSONResponse{
				Message: ptr("Unauthorized: invalid Authorization header"),
			},
		}, nil
	}

	clusterKubeconfig, err := s.getClusterKubeconfig(ctx, namespace, request.Name)
	if err != nil {
		slog.Error("failed to get kubeconfig", "error", err)
		return api.GetV2ClustersNameKubeconfigs404JSONResponse{
			N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{
				Message: ptr("404 Not Found: kubeconfig not found"),
			},
		}, nil
	}

	// Determine TTL for kubeconfig JWT based on configuration
	//    > 0: request a renewed token with specified TTL
	//   == 0: skip renewal (pass-through existing token)
	var kubeconfigTTL *time.Duration
	// always set pointer (including zero) so tokenRenewal can distinguish between "no config provided" (nil) and 0 meaning skip
	if s.config != nil {
		tmp := s.config.DefaultKubeconfigTTL
		kubeconfigTTL = &tmp
	}

	clusterKubeconfigUpdated, err := updateKubeconfigWithTokenFunc(clusterKubeconfig, namespace, request.Name, request.Params.Authorization, s.config.DisableAuth, kubeconfigTTL)
	if err != nil {
		if strings.Contains(err.Error(), "token expired") || strings.Contains(err.Error(), "token not renewable") {
			slog.Warn("authorization token rejected", "reason", err.Error())
			return api.GetV2ClustersNameKubeconfigs401JSONResponse{
				N401UnauthorizedJSONResponse: api.N401UnauthorizedJSONResponse{Message: ptr("Unauthorized: token expired")},
			}, nil
		}
		slog.Error("failed to update kubeconfig with token", "error", err)
		return api.GetV2ClustersNameKubeconfigs500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("500 Internal Server Error: failed to process kubeconfig"),
			},
		}, nil
	}

	return api.GetV2ClustersNameKubeconfigs200JSONResponse{Kubeconfig: ptr(clusterKubeconfigUpdated)}, nil
}

func (s *Server) getClusterKubeconfig(ctx context.Context, namespace, clusterName string) (kubeconfigParameters, error) {
	if s.config == nil {
		return kubeconfigParameters{}, fmt.Errorf("config is nil")
	}

	unstructuredClusterSecret, err := s.k8sclient.Resource(core.SecretResourceSchema).
		Namespace(namespace).Get(ctx, fmt.Sprintf("%s-kubeconfig", clusterName), metav1.GetOptions{})
	if err != nil || unstructuredClusterSecret == nil {
		return kubeconfigParameters{}, fmt.Errorf("failed to get kubeconfig secret: %w", err)
	}

	dataValue, found, err := unstructured.NestedString(unstructuredClusterSecret.Object, "data", "value")
	if err != nil {
		return kubeconfigParameters{}, fmt.Errorf("failed to get raw kubeconfig data from secret: %w", err)
	}
	if !found {
		return kubeconfigParameters{}, fmt.Errorf("kubeconfig data not found in secret")
	}

	kubeconfigBytes, err := base64.StdEncoding.DecodeString(dataValue)
	if err != nil {
		return kubeconfigParameters{}, fmt.Errorf("failed to decode kubeconfig data: %w", err)
	}

	var caDataInSecretValue string
	apiServerCA, found, err := unstructured.NestedString(unstructuredClusterSecret.Object, "data", "apiServerCA")
	if err != nil || !found {
		slog.Warn("failed to get apiServerCA from secret", "namespace", namespace, "name", clusterName, "error", err)

		caData, err := unmarshalKubeconfig(string(kubeconfigBytes))
		if err != nil {
			return kubeconfigParameters{}, err
		}

		caDataInSecretValue, err = getCertificateAuthorityData(caData)
		if err != nil {
			return kubeconfigParameters{}, err
		}

		return kubeconfigParameters{serverCA: caDataInSecretValue, clusterDomain: s.config.ClusterDomain, userName: s.config.Username, kubeConfigDecode: string(kubeconfigBytes)}, nil

	}

	return kubeconfigParameters{serverCA: apiServerCA, clusterDomain: s.config.ClusterDomain, userName: s.config.Username, kubeConfigDecode: string(kubeconfigBytes)}, nil
}

// server internal kubeconfig from secrets:
// server: http://edge-connect-gateway-cluster-connect-gateway.orch-cluster.svc:8080/kubernetes/<project-id>-<cluster-name>
// server external:
// server: https://connect-gateway.<domain>:443/kubernetes/<project-id>-<cluster-name>

func updateKubeconfigWithToken(kubeconfig kubeconfigParameters, namespace, clusterName, authHeader string, disableAuth bool, ttl *time.Duration) (string, error) {
	token := auth.GetAccessToken(authHeader)
	newAccessToken, err := tokenRenewalFunc(token, disableAuth, ttl)
	if err != nil {
		return "", err
	}
	caData, domain, userName := kubeconfig.serverCA, kubeconfig.clusterDomain, kubeconfig.userName

	config, err := unmarshalKubeconfig(kubeconfig.kubeConfigDecode)
	if err != nil {
		return "", err
	}

	serverUrl, err := extractServerURL(kubeconfig.kubeConfigDecode)
	if err != nil {
		return "", err
	}

	middleUrl := fmt.Sprintf("/kubernetes/%s-%s", namespace, clusterName)

	endSegment, err := extractEndSegment(serverUrl, middleUrl)
	if err != nil {
		return "", err
	}

	serverAddress := fmt.Sprintf("https://connect-gateway.%s:443%s%s", domain, middleUrl, endSegment)
	slog.Debug("serverAddress", "decoded", serverAddress)

	updateKubeconfigFields(config, clusterName+"-"+userName, clusterName, serverAddress, caData, newAccessToken)

	updatedKubeconfig, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated kubeconfig: %w", err)
	}

	return string(updatedKubeconfig), nil
}

func unmarshalKubeconfig(kubeconfig string) (map[string]interface{}, error) {
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(kubeconfig), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubeconfig: %w", err)
	}

	return config, nil
}

func extractServerURL(kubeconfigData string) (string, error) {
	var config kubeConfigData
	if err := yaml.Unmarshal([]byte(kubeconfigData), &config); err != nil {
		return "", fmt.Errorf("failed to parse kubeconfig data: %v", err)
	}

	if len(config.Clusters) == 0 {
		return "", fmt.Errorf("no clusters found in kubeconfig")
	}

	serverURL := config.Clusters[0].Cluster.Server

	return serverURL, nil
}

// getCertificateAuthorityData extracts the certificate-authority-data from the data values in kubeconfig secret
func getCertificateAuthorityData(config map[string]interface{}) (string, error) {
	clusters, ok := config["clusters"].([]interface{})
	if !ok || len(clusters) == 0 {
		return "", fmt.Errorf("invalid kubeconfig: missing clusters")
	}

	cluster, ok := clusters[0].(map[interface{}]interface{})
	if !ok {
		return "", fmt.Errorf("invalid kubeconfig: cluster is not a map")
	}

	clusterData, ok := cluster["cluster"].(map[interface{}]interface{})
	if !ok {
		return "", fmt.Errorf("invalid kubeconfig: cluster data is not a map")
	}

	caData, ok := clusterData["certificate-authority-data"]
	if !ok {
		return "", fmt.Errorf("failed to get certificate-authority-data from kubeconfig")
	}

	caDataInSecretValue, ok := caData.(string)
	if !ok {
		return "", fmt.Errorf("failed to assert caData to string")
	}

	return caDataInSecretValue, nil
}

func updateKubeconfigFields(config map[string]interface{}, user, clusterName, serverAddress string, caData, token interface{}) {
	config["apiVersion"] = "v1"
	config["kind"] = "Config"
	config["clusters"] = []map[string]interface{}{
		{
			"name": clusterName,
			"cluster": map[string]interface{}{
				"server":                     serverAddress,
				"certificate-authority-data": caData,
			},
		},
	}

	config["users"] = []map[string]interface{}{
		{
			"name": user,
			"user": map[string]interface{}{
				"token": token,
			},
		},
	}

	config["contexts"] = []map[string]interface{}{
		{
			"name": user + "@" + clusterName,
			"context": map[string]interface{}{
				"user":    user,
				"cluster": clusterName,
			},
		},
	}
}

var (
	keycloakClientTTLEnforced bool
)

func tokenRenewal(accessToken string, disableAuth bool, ttl *time.Duration) (string, error) {
	// skip renewal when auth disabled or configured TTL is exactly zero
	if disableAuth {
		slog.Debug("authentication disabled, skipping token renewal")
		return accessToken, nil
	}
	// If ttl pointer provided (including 0) attempt one-time Keycloak TTL enforcement/clear BEFORE deciding to skip.
	// Use an admin (M2M) token instead of the incoming access token to avoid 401 when the user token lacks realm-management roles.
	if ttl != nil && !keycloakClientTTLEnforced {
		issuer := os.Getenv(auth.OidcUrlEnvVar)
		if issuer == "" {
			issuer = os.Getenv("KEYCLOAK_URL")
		}
		if err := auth.EnsureM2MCredentials(false); err != nil {
			slog.Warn("cannot ensure M2M credentials for TTL enforcement", "error", err)
		} else {
			clientID := auth.GetM2MClientID()
			// obtain an admin token (use default lifetime; override enforcement applies to client settings, not the admin token itself)
			adminToken, errTok := JwtTokenWithM2MFunc(context.Background(), nil)
			if errTok != nil {
				slog.Warn("failed to get M2M admin token for TTL enforcement", "error", errTok)
			} else if issuer != "" && clientID != "" {
				if *ttl > 0 {
					auth.EnforceClientAccessTokenTTL(context.Background(), issuer, "", clientID, *ttl, adminToken, slog.Default())
				} else { // clear override when ttl == 0 to revert to realm default
					auth.ClearClientAccessTokenTTL(context.Background(), issuer, "", clientID, adminToken, slog.Default())
				}
				// mark enforced to avoid repeated admin calls (future enhancement: track last applied TTL to allow dynamic changes without restart)
				keycloakClientTTLEnforced = true
			}
		}
	}
	if ttl != nil && *ttl == 0 {
		slog.Debug("configured kubeconfig TTL == 0 (inherit realm) after enforcement, skipping token renewal")
		return accessToken, nil
	}

	// parse claims (without signature verification – existing ExtractClaims behavior) to inspect exp.
	_, _, exp, err := auth.ExtractClaims(accessToken)
	if err != nil {
		return "", fmt.Errorf("token not renewable: %w", err)
	}

	// this avoid renew an expired token
	if time.Now().After(exp) {
		return "", fmt.Errorf("token expired at %s", exp.UTC().Format(time.RFC3339))
	}

	ctx := context.Background()

	// one-time attempt to enforce / clear Keycloak per-client TTL using the service account token itself
	// proceed if we have a desired ttl pointer (could be zero). We treat zero as 'inherit realm'
	newToken, err := JwtTokenWithM2MFunc(ctx, ttl)
	if err != nil {
		return "", fmt.Errorf("failed to get new M2M token: %w", err)
	}

	// allows service tokens based on groups + roles
	newAzp, newUser, newExp, claimErr := auth.ExtractClaims(newToken)
	if claimErr != nil {
		slog.Warn("failed to parse renewed token claims; falling back to original token", "error", claimErr)

		return accessToken, nil
	}

	remainingOriginal := time.Until(exp)
	requestedTTL := "<nil>"
	if ttl != nil {
		requestedTTL = ttl.String()
	}

	renewedLifetime := time.Until(newExp)
	slog.Debug("kubeconfig token renewed", "original_remaining", remainingOriginal, "requested_ttl", requestedTTL, "renewed_lifetime", renewedLifetime, "user", newUser, "azp", newAzp)

	return newToken, nil
}

type kubeConfigData struct {
	Clusters []struct {
		Cluster struct {
			Server string `yaml:"server"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
}

type kubeconfigParameters struct {
	serverCA         string
	clusterDomain    string
	userName         string
	kubeConfigDecode string
}

// url - intersection = endSegment
func extractEndSegment(url, intersection string) (string, error) {
	index := strings.Index(url, intersection)
	if index == -1 {
		return "", fmt.Errorf("known part not found in URL")
	}

	startIndex := index + len(intersection)
	endSegment := url[startIndex:]

	return endSegment, nil
}
