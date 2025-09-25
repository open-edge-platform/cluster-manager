// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// NewVaultAuthFunc allows tests to inject a mock VaultAuth implementation
var NewVaultAuthFunc = NewVaultAuth

// cached M2M client credentials (populated at startup to avoid Vault lookups per token request)
var cachedClientID string
var cachedClientSecret string
var credsMu sync.Mutex

// SetCachedM2MCredentials allows the main package (or tests) to preload client credentials so that
// JwtTokenWithM2M does not need to contact Vault on each invocation. Safe for concurrent reads after set
func SetCachedM2MCredentials(id, secret string) {
	credsMu.Lock()
	cachedClientID = id
	cachedClientSecret = secret
	credsMu.Unlock()
}

// ensureM2MCredentials loads credentials from Vault if cache empty or forceRefresh requested
// returns error if access fails
func ensureM2MCredentials(ctx context.Context, forceRefresh bool) error {
	credsMu.Lock()
	idEmpty := cachedClientID == "" || cachedClientSecret == ""
	credsMu.Unlock()

	if !forceRefresh && !idEmpty {
		return nil
	}

	vaultAuth, err := NewVaultAuthFunc(VaultServer, ServiceAccount)
	if err != nil {
		return fmt.Errorf("failed to create vault auth: %w", err)
	}

	id, secret, err := vaultAuth.GetClientCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch M2M credentials from Vault: %w", err)
	}

	SetCachedM2MCredentials(id, secret)
	if forceRefresh {
		slog.Warn("token failure - M2M credentials refreshed from Vault")
	} else {
		slog.Debug("loaded M2M credentials from Vault)")
	}
	return nil
}

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
	if err := ensureM2MCredentials(ctx, false); err != nil {
		return "", err
	}

	credsMu.Lock()
	clientID, clientSecret := cachedClientID, cachedClientSecret
	credsMu.Unlock()

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	if keycloakURL == "" { // use OIDC server when KEYCLOAK_URL isn't available
		keycloakURL = os.Getenv(OidcUrlEnvVar)
	}

	if keycloakURL == "" {
		return "", fmt.Errorf("KEYCLOAK_URL (or %s) environment variable not set", OidcUrlEnvVar)
	}

	// prepare for M2M token request
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

	// attempt credential refresh & retry once
	accessToken, retryable, err := doM2MTokenRequest(client, req)
	if err != nil && retryable {		
		if errRef := ensureM2MCredentials(ctx, true); errRef == nil {
			credsMu.Lock()
			clientID, clientSecret = cachedClientID, cachedClientSecret
			credsMu.Unlock()

			data.Set("client_id", clientID)
			data.Set("client_secret", clientSecret)

			req2, r2err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewBufferString(data.Encode()))
			if r2err != nil {
				return "", fmt.Errorf("failed to create retry token request: %w", r2err)
			}

			req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			client = &http.Client{Timeout: 10 * time.Second}

			return doFinalTokenRequest(client, req2)
		}
	}

	return accessToken, err
}

// doM2MTokenRequest performs the token request, returning (token, retryable, error)
func doM2MTokenRequest(client *http.Client, req *http.Request) (string, bool, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("failed to perform token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		body := strings.TrimSpace(string(bodyBytes))
		// retryable is true if credentials aree invalid or rotated
		retryable := resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest && (strings.Contains(strings.ToLower(body), "invalid_client") || strings.Contains(strings.ToLower(body), "unauthorized"))
		if retryable {
			slog.Warn("M2M token request failed - trying credential refresh", "status", resp.StatusCode, "body", body, "url", req.URL.Redacted())
		} else {
			slog.Error("M2M token request failed", "status", resp.StatusCode, "body", body, "url", req.URL.Redacted())
		}

		return "", retryable, fmt.Errorf("failed to get M2M token, status code: %d", resp.StatusCode)
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", false, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return "", true, errors.New("empty access token in response")
	}

	return tokenResponse.AccessToken, false, nil
}

// doFinalTokenRequest executes a retry after credentials refresh. no additional retries to avoid loops
func doFinalTokenRequest(client *http.Client, req *http.Request) (string, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform retry token request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		body := strings.TrimSpace(string(bodyBytes))
		slog.Error("M2M token retry failed", "status", resp.StatusCode, "body", body, "url", req.URL.Redacted())

		return "", fmt.Errorf("retry failed; status code: %d", resp.StatusCode)
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode retry token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return "", errors.New("empty access token in retry response")
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
