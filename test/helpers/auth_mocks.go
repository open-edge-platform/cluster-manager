// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MockVaultAuth implements VaultAuth interface for testing
type MockVaultAuth struct {
	ClientID     string
	ClientSecret string
	ShouldFail   bool
	FailMessage  string
}

func NewMockVaultAuth(clientID, clientSecret string) *MockVaultAuth {
	return &MockVaultAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		ShouldFail:   false,
	}
}

func (m *MockVaultAuth) GetClientCredentials(ctx context.Context) (string, string, error) {
	if m.ShouldFail {
		return "", "", fmt.Errorf("mock vault error: %s", m.FailMessage)
	}
	return m.ClientID, m.ClientSecret, nil
}

func (m *MockVaultAuth) SetFailure(shouldFail bool, message string) {
	m.ShouldFail = shouldFail
	m.FailMessage = message
}

// MockKeycloakServer provides a mock Keycloak server for testing
type MockKeycloakServer struct {
	Server    *httptest.Server
	TokenTTL  time.Duration
	UserRoles []string
}

func NewMockKeycloakServer() *MockKeycloakServer {
	mock := &MockKeycloakServer{
		TokenTTL:  1 * time.Hour, // Default TTL
		UserRoles: []string{"default-role"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/protocol/openid-connect/token", mock.handleTokenRequest)
	mock.Server = httptest.NewServer(mux)

	return mock
}

func (m *MockKeycloakServer) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

func (m *MockKeycloakServer) URL() string {
	return m.Server.URL
}

func (m *MockKeycloakServer) SetTokenTTL(ttl time.Duration) {
	m.TokenTTL = ttl
}

func (m *MockKeycloakServer) SetUserRoles(roles []string) {
	m.UserRoles = roles
}

func (m *MockKeycloakServer) handleTokenRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	if grantType != "client_credentials" && grantType != "password" {
		http.Error(w, "Unsupported grant type", http.StatusBadRequest)
		return
	}

	// Generate a mock JWT token with the requested TTL
	token := m.generateMockJWT(m.TokenTTL)

	response := map[string]interface{}{
		"access_token":  token, // This should now be a real JWT
		"refresh_token": "mock-refresh-token",
		"token_type":    "Bearer",
		"expires_in":    int(m.TokenTTL.Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode mock token response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (m *MockKeycloakServer) generateMockJWT(ttl time.Duration) string {
	now := time.Now()
	exp := now.Add(ttl)

	claims := jwt.MapClaims{
		"iss":                "http://platform-keycloak.orch-platform.svc/realms/master",
		"azp":                "test-client",
		"preferred_username": "test-user",
		"exp":                exp.Unix(),
		"iat":                now.Unix(),
		"realm_access": map[string]interface{}{
			"roles": m.UserRoles,
		},
	}

	// Get the dynamically generated test private key
	privateKey, err := GetTestPrivateKey()
	if err != nil {
		panic(fmt.Sprintf("failed to get test private key: %v", err))
	}

	// Get the dynamically generated key ID
	keyID, err := GetTestKeyID()
	if err != nil {
		panic(fmt.Sprintf("failed to get test key ID: %v", err))
	}

	// Sign with PS512 (RSA-PSS with SHA-512)
	token := jwt.NewWithClaims(jwt.SigningMethodPS512, claims)
	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		panic(fmt.Sprintf("failed to sign token: %v", err))
	}

	return tokenString
} // Helper functions for testing

// CreateTestJWT creates a JWT token for testing with specified expiration and roles
func CreateTestJWT(exp time.Time, roles []string) string {
	claims := jwt.MapClaims{
		"iss":                "http://platform-keycloak.orch-platform.svc/realms/master",
		"azp":                "test-client",
		"preferred_username": "test-user",
		"exp":                exp.Unix(),
		"iat":                time.Now().Unix(),
		"realm_access": map[string]interface{}{
			"roles": roles,
		},
	}

	// Get the dynamically generated test private key
	privateKey, err := GetTestPrivateKey()
	if err != nil {
		panic(fmt.Sprintf("failed to get test private key: %v", err))
	}

	// Get the dynamically generated key ID
	keyID, err := GetTestKeyID()
	if err != nil {
		panic(fmt.Sprintf("failed to get test key ID: %v", err))
	}

	// Sign with PS512 (RSA-PSS with SHA-512)
	token := jwt.NewWithClaims(jwt.SigningMethodPS512, claims)
	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		panic(fmt.Sprintf("failed to sign token: %v", err))
	}

	return tokenString
}

// ExtractTokenTTL extracts the TTL from a JWT token for testing
func ExtractTokenTTL(tokenString string) (time.Duration, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("failed to extract claims")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return 0, fmt.Errorf("invalid or missing exp claim")
	}

	expTime := time.Unix(int64(exp), 0)
	ttl := time.Until(expTime)
	return ttl, nil
}

// ValidateKubeconfigToken validates that a kubeconfig contains a token with expected TTL
func ValidateKubeconfigToken(kubeconfigYAML string, expectedTTL time.Duration, tolerance time.Duration) error {
	// Extract token from kubeconfig YAML
	lines := strings.Split(kubeconfigYAML, "\n")
	var token string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "token:") {
			token = strings.TrimSpace(strings.TrimPrefix(line, "token:"))
			break
		}
	}

	if token == "" {
		return fmt.Errorf("no token found in kubeconfig")
	}

	// Extract TTL from token
	actualTTL, err := ExtractTokenTTL(token)
	if err != nil {
		return fmt.Errorf("failed to extract TTL from token: %w", err)
	}

	// Check if TTL is within tolerance
	diff := actualTTL - expectedTTL
	if diff < 0 {
		diff = -diff
	}

	if diff > tolerance {
		return fmt.Errorf("token TTL %v differs from expected %v by more than tolerance %v",
			actualTTL, expectedTTL, tolerance)
	}

	return nil
}
