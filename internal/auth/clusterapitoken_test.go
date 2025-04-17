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
