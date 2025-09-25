// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// NewVaultAuthFunc allows tests to inject a mock VaultAuth implementation.
// In production it points to NewVaultAuth.
var NewVaultAuthFunc = NewVaultAuth

// ExtractClaims extracts claims from a JWT token
func ExtractClaims(tokenString string) (string, string, time.Time, error) {
	// Parse the token without verifying the signature to extract the claims
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", time.Time{}, fmt.Errorf("failed to extract claims")
	}

	azp, _ := claims["azp"].(string) // authorized party
	preferredUsername, _ := claims["preferred_username"].(string)

	exp, ok := claims["exp"].(float64) // expire time
	if !ok {
		return azp, preferredUsername, time.Time{}, fmt.Errorf("invalid or missing exp claim")
	}

	expirationTime := time.Unix(int64(exp), 0) // convert to time.Time

	return azp, preferredUsername, expirationTime, nil
}

// JwtTokenWithM2M retrieves a new token from Keycloak using M2M authentication with configurable TTL
func JwtTokenWithM2M(ctx context.Context, ttl *time.Duration) (string, error) {
	defaultTTL := 1 * time.Hour
	if ttl == nil {
		ttl = &defaultTTL
	}

	// Get M2M credentials
	vaultAuth, err := NewVaultAuthFunc(VaultServer, ServiceAccount)
	if err != nil {
		return "", fmt.Errorf("failed to create vault auth: %w", err)
	}

	clientID, clientSecret, err := vaultAuth.GetClientCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get M2M credentials from Vault: %w", err)
	}

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	if keycloakURL == "" { // use OIDC server when KEYCLOAK_URL isn't available
		keycloakURL = os.Getenv(OidcUrlEnvVar)
	}
	if keycloakURL == "" {
		return "", fmt.Errorf("KEYCLOAK_URL (or %s) environment variable not set", OidcUrlEnvVar)
	}

	// Prepare M2M token request
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	tokenURL := fmt.Sprintf("%s/protocol/openid-connect/token", keycloakURL)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		body := strings.TrimSpace(string(bodyBytes))
		slog.Error("M2M token request failed", "status", resp.StatusCode, "body", body, "url", resp.Request.URL.Redacted())
		return "", fmt.Errorf("failed to get M2M token, status code: %d", resp.StatusCode)
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResponse.AccessToken, nil
}

// ExtractUserRoles extracts user roles from JWT token claims
func ExtractUserRoles(claims jwt.MapClaims) ([]string, error) {
	realmAccess, exists := claims["realm_access"]
	if !exists {
		return nil, fmt.Errorf("realm_access not found in token claims")
	}

	realmAccessMap, ok := realmAccess.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("realm_access is not a valid map")
	}

	rolesInterface, exists := realmAccessMap["roles"]
	if !exists {
		return nil, fmt.Errorf("roles not found in realm_access")
	}

	rolesSlice, ok := rolesInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("roles is not a valid slice")
	}

	var roles []string
	for _, role := range rolesSlice {
		roleStr, ok := role.(string)
		if !ok {
			return nil, fmt.Errorf("role is not a string")
		}
		roles = append(roles, roleStr)
	}

	// return an empty slice instead of nil for no roles
	if roles == nil {
		roles = []string{}
	}

	return roles, nil
}
