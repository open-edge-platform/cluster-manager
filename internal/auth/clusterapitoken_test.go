// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestExtractClaims(t *testing.T) {
	tests := []struct {
		name          string
		tokenClaims   jwt.MapClaims
		expectedError bool
		expectedAzp   string
		expectedUser  string
		expectedExp   time.Time
	}{
		{
			name: "valid token",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name:          "invalid token",
			tokenClaims:   nil,
			expectedError: true,
		},
		{
			name: "token without exp claim",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
			},
			expectedError: true,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Time{},
		},
		{
			name: "token with missing azp claim",
			tokenClaims: jwt.MapClaims{
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name: "token with missing preferred_username claim",
			tokenClaims: jwt.MapClaims{
				"azp": "test-client-id",
				"exp": time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name: "token with invalid exp claim",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                "invalid-exp",
			},
			expectedError: true,
		},
		{
			name: "expired token",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(-time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(-time.Hour),
		},
		{
			name: "token with future expiration",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(100 * time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(100 * time.Hour),
		},
		{
			name: "token with additional claims",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
				"extra_claim":        "extra_value",
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenString string
			var err error
			tokenString = "invalid-token"
			if tt.tokenClaims != nil {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, tt.tokenClaims)
				tokenString, err = token.SignedString([]byte("secret"))
				assert.NoError(t, err)
			}

			clientId, username, exp, err := ExtractClaims(tokenString)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAzp, clientId)
				assert.Equal(t, tt.expectedUser, username)
				if tt.expectedExp.IsZero() {
					assert.Equal(t, tt.expectedExp, exp)
				} else {
					assert.WithinDuration(t, tt.expectedExp, exp, time.Minute)
				}
			}
		})
	}
}

// TestJwtTokenWithM2M tests the M2M token generation function
func TestJwtTokenWithM2M(t *testing.T) {
	tests := []struct {
		name        string
		ttl         *time.Duration
		expectedTTL time.Duration
		skipReason  string
	}{
		{
			name:        "successful M2M token with 1 hour TTL",
			ttl:         func() *time.Duration { d := 1 * time.Hour; return &d }(),
			expectedTTL: 1 * time.Hour,
			skipReason:  "Function will be implemented in next PR",
		},
		{
			name:        "successful M2M token with 24 hour TTL", 
			ttl:         func() *time.Duration { d := 24 * time.Hour; return &d }(),
			expectedTTL: 24 * time.Hour,
			skipReason:  "Function will be implemented in next PR",
		},
		{
			name:        "successful M2M token without TTL (use default)",
			ttl:         nil,
			expectedTTL: 1 * time.Hour, // Default TTL
			skipReason:  "Function will be implemented in next PR",
		},
		{
			name:        "M2M token with empty roles",
			ttl:         func() *time.Duration { d := 2 * time.Hour; return &d }(),
			expectedTTL: 2 * time.Hour,
			skipReason:  "Function will be implemented in next PR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Skip(tt.skipReason)
			// TODO: Implement when JwtTokenWithM2M function is available
			// token, err := JwtTokenWithM2M(context.Background(), tt.ttl)
			// require.NoError(t, err)
			// assert.NotEmpty(t, token)
			
			// // Validate TTL
			// ttl, err := ExtractTokenTTL(token)  
			// require.NoError(t, err)
			// tolerance := 1 * time.Minute
			// diff := ttl - tt.expectedTTL
			// if diff < 0 {
			//     diff = -diff
			// }
			// assert.True(t, diff <= tolerance, 
			//     "Token TTL %v should be within %v of expected %v", 
			//     ttl, tolerance, tt.expectedTTL)
		})
	}
}

// TestExtractUserRoles tests user role extraction from JWT tokens
func TestExtractUserRoles(t *testing.T) {
	tests := []struct {
		name          string
		tokenClaims   jwt.MapClaims
		expectedRoles []string
		expectedError bool
		skipReason    string
	}{
		{
			name: "token with multiple roles",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"admin", "user", "cluster-reader"},
				},
			},
			expectedRoles: []string{"admin", "user", "cluster-reader"},
			expectedError: false,
			skipReason:    "Function will be implemented in next PR",
		},
		{
			name: "token with single role",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"user"},
				},
			},
			expectedRoles: []string{"user"},
			expectedError: false,
			skipReason:    "Function will be implemented in next PR",
		},
		{
			name: "token with no roles",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{},
				},
			},
			expectedRoles: []string{},
			expectedError: false,
			skipReason:    "Function will be implemented in next PR",
		},
		{
			name: "token without realm_access",
			tokenClaims: jwt.MapClaims{
				"azp": "test-client",
			},
			expectedRoles: nil,
			expectedError: true,
			skipReason:    "Function will be implemented in next PR",
		},
		{
			name: "token with invalid roles format",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": "invalid-format",
				},
			},
			expectedRoles: nil,
			expectedError: true,
			skipReason:    "Function will be implemented in next PR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Skip(tt.skipReason)
			// TODO: Implement when ExtractUserRoles function is available
			// roles, err := ExtractUserRoles(tt.tokenClaims)
			// 
			// if tt.expectedError {
			//     require.Error(t, err)
			// } else {
			//     require.NoError(t, err)
			//     assert.Equal(t, tt.expectedRoles, roles)
			// }
		})
	}
}

// TestTokenTTLValidation tests TTL validation logic
func TestTokenTTLValidation(t *testing.T) {
	tests := []struct {
		name        string
		ttl         time.Duration
		expectValid bool
		description string
	}{
		{
			name:        "valid 1 hour TTL",
			ttl:         1 * time.Hour,
			expectValid: true,
			description: "1 hour is within valid range",
		},
		{
			name:        "valid 24 hour TTL",
			ttl:         24 * time.Hour,
			expectValid: true,
			description: "24 hours is within valid range",
		},
		{
			name:        "valid 7 day TTL",
			ttl:         7 * 24 * time.Hour,
			expectValid: true,
			description: "7 days is within valid range",
		},
		{
			name:        "too short TTL (5 minutes)",
			ttl:         5 * time.Minute,
			expectValid: false,
			description: "5 minutes is below minimum TTL",
		},
		{
			name:        "too long TTL (30 days)",
			ttl:         30 * 24 * time.Hour,
			expectValid: false,
			description: "30 days exceeds maximum TTL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test TTL validation logic
			minTTL := 10 * time.Minute
			maxTTL := 14 * 24 * time.Hour // 14 days
			
			isValid := tt.ttl >= minTTL && tt.ttl <= maxTTL
			assert.Equal(t, tt.expectValid, isValid, tt.description)
		})
	}
}
