// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockVaultAuth(t *testing.T) {
	tests := []struct {
		name          string
		clientID      string
		clientSecret  string
		shouldFail    bool
		failMessage   string
		expectedError bool
	}{
		{
			name:          "successful credential retrieval",
			clientID:      "test-client",
			clientSecret:  "test-secret",
			shouldFail:    false,
			expectedError: false,
		},
		{
			name:          "vault failure",
			clientID:      "",
			clientSecret:  "",
			shouldFail:    true,
			failMessage:   "vault connection failed",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockVaultAuth(tt.clientID, tt.clientSecret)
			if tt.shouldFail {
				mock.SetFailure(true, tt.failMessage)
			}

			clientID, clientSecret, err := mock.GetClientCredentials(context.Background())

			if tt.expectedError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.failMessage)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.clientID, clientID)
				assert.Equal(t, tt.clientSecret, clientSecret)
			}
		})
	}
}

func TestMockKeycloakServer(t *testing.T) {
	server := NewMockKeycloakServer()
	defer server.Close()

	// Test default settings
	assert.Equal(t, 1*time.Hour, server.TokenTTL)
	assert.Equal(t, []string{"default-role"}, server.UserRoles)
	assert.NotEmpty(t, server.URL())

	// Test setting custom TTL
	server.SetTokenTTL(2 * time.Hour)
	assert.Equal(t, 2*time.Hour, server.TokenTTL)

	// Test setting custom roles
	customRoles := []string{"admin", "user", "cluster-reader"}
	server.SetUserRoles(customRoles)
	assert.Equal(t, customRoles, server.UserRoles)
}

func TestCreateTestJWT(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour)
	roles := []string{"admin", "user"}

	token := CreateTestJWT(exp, roles)
	assert.NotEmpty(t, token)

	// Test TTL extraction
	ttl, err := ExtractTokenTTL(token)
	require.NoError(t, err)

	// Should be close to 2 hours (within 1 minute tolerance)
	expectedTTL := 2 * time.Hour
	tolerance := 1 * time.Minute
	diff := ttl - expectedTTL
	if diff < 0 {
		diff = -diff
	}
	assert.True(t, diff <= tolerance, "TTL %v should be within %v of expected %v", ttl, tolerance, expectedTTL)
}

func TestExtractTokenTTL(t *testing.T) {
	tests := []struct {
		name          string
		expiration    time.Time
		expectedError bool
	}{
		{
			name:          "valid token with future expiration",
			expiration:    time.Now().Add(1 * time.Hour),
			expectedError: false,
		},
		{
			name:          "valid token with different expiration",
			expiration:    time.Now().Add(24 * time.Hour),
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := CreateTestJWT(tt.expiration, []string{"test-role"})

			ttl, err := ExtractTokenTTL(token)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Calculate expected TTL
				expectedTTL := time.Until(tt.expiration)
				tolerance := 1 * time.Minute

				diff := ttl - expectedTTL
				if diff < 0 {
					diff = -diff
				}
				assert.True(t, diff <= tolerance,
					"Extracted TTL %v should be within %v of expected %v",
					ttl, tolerance, expectedTTL)
			}
		})
	}
}

func TestValidateKubeconfigToken(t *testing.T) {
	// Create a test kubeconfig with a token
	exp := time.Now().Add(2 * time.Hour)
	token := CreateTestJWT(exp, []string{"test-role"})

	kubeconfigYAML := `apiVersion: v1
kind: Config
users:
- name: test-user
  user:
    token: ` + token + `
`

	tests := []struct {
		name          string
		expectedTTL   time.Duration
		tolerance     time.Duration
		expectedError bool
	}{
		{
			name:          "valid TTL within tolerance",
			expectedTTL:   2 * time.Hour,
			tolerance:     5 * time.Minute,
			expectedError: false,
		},
		{
			name:          "TTL outside tolerance",
			expectedTTL:   1 * time.Hour, // Token has 2 hours
			tolerance:     30 * time.Minute,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKubeconfigToken(kubeconfigYAML, tt.expectedTTL, tt.tolerance)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateKubeconfigTokenWithoutToken(t *testing.T) {
	kubeconfigYAML := `apiVersion: v1
kind: Config
users:
- name: test-user
  user:
    username: test
`

	err := ValidateKubeconfigToken(kubeconfigYAML, 1*time.Hour, 1*time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no token found in kubeconfig")
}
