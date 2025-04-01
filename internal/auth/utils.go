// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang-jwt/jwt/v5"
)

const (
	AuthorizationHeaderKey   = "Authorization"
	ActiveProjectIdHeaderKey = "Activeprojectid"
	BearerPrefix             = "Bearer "
)

func getWellKnownConfig(client *http.Client, endpoint string) (*oidcProviderConfig, error) {
	configEndpoint, err := url.JoinPath(endpoint, oidConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error joining url: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, configEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting oidc well known config: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("error getting oidc well known config: empty response")
	}
	defer resp.Body.Close()

	configJson, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body of the oidc config: %w", err)
	}

	var config oidcProviderConfig
	if err := json.Unmarshal(configJson, &config); err != nil {
		return nil, fmt.Errorf("error unmarshalling oidc config: %w", err)
	}

	return &config, nil
}

// extractRolesFromToken extracts roles from the JWT token.
func extractRolesFromToken(token *jwt.Token) ([]string, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	realmAccess, ok := claims["realm_access"].(map[string]interface{})
	if !ok {
		return nil, errors.New("realm_access claim is missing or invalid")
	}

	rolesInterface, ok := realmAccess["roles"].([]interface{})
	if !ok {
		return nil, errors.New("roles are missing or invalid in realm_access claim")
	}

	roles := make([]string, 0, len(rolesInterface))
	for _, r := range rolesInterface {
		role, ok := r.(string)
		if !ok {
			continue // ignore non-string roles
		}

		roles = append(roles, role)
	}

	return roles, nil
}

// getAuthHeader returns the 'Authorization' header from the request
func getAuthHeader(req *http.Request) string {
	auth := req.Header.Get(AuthorizationHeaderKey)
	if auth == "" {
		auth = req.Header.Get(strings.ToLower(AuthorizationHeaderKey))
	}
	return auth
}

// getAuthHeader returns the 'Authorization' header from the request
func getProjectHeader(req *http.Request) string {
	return req.Header.Get(ActiveProjectIdHeaderKey)
}

func newAuthError(input *openapi3filter.AuthenticationInput, err error) error {
	path := "unset"
	if input.RequestValidationInput.Request.URL != nil {
		path = input.RequestValidationInput.Request.URL.Path
	}

	slog.Error("request failed authentication/authorization", "method", input.RequestValidationInput.Request.Method, "path", path, "error", err.Error())
	return input.NewError(errors.New("unauthorized"))
}

// GetAccessToken returns the access token from the Authorization header
func GetAccessToken(authHeader string) string {
	return strings.TrimPrefix(authHeader, "Bearer ")
}
