// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/open-edge-platform/cluster-manager/internal/auth"
)

func TestAuthN(t *testing.T) {
	kid := "test-key"

	validMethod := jwt.SigningMethodPS512
	invalidMethod := jwt.SigningMethodPS256

	validClaims := jwt.MapClaims{"iss": "not-empty"}
	noIssClaims := jwt.MapClaims{}
	expiredClaims := jwt.MapClaims{"iss": "not-empty", "exp": 1}
	notBeforeClaims := jwt.MapClaims{"iss": "not-empty", "nbf": time.Now().Add(time.Minute).Unix()}

	validHeader := map[string]interface{}{"kid": kid}
	noKidHeader := map[string]interface{}{}

	validToken, validTokenPublicKey := signToken(t, newToken(validMethod, validHeader, validClaims))
	invalidMethodToken, _ := signToken(t, newToken(invalidMethod, validHeader, validClaims))
	noIssClaimsToken, _ := signToken(t, newToken(validMethod, validHeader, noIssClaims))
	noKidHeaderToken, _ := signToken(t, newToken(validMethod, noKidHeader, validClaims))
	expiredClaimsToken, expiredClaimsTokenPublicKey := signToken(t, newToken(validMethod, validHeader, expiredClaims))
	notBeforeClaimsToken, notBeforeClaimsTokenPublicKey := signToken(t, newToken(validMethod, validHeader, notBeforeClaims))

	constructInput := func(token string) *openapi3filter.AuthenticationInput {
		return &openapi3filter.AuthenticationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: &http.Request{
				Header: http.Header{auth.AuthorizationHeaderKey: {token}},
			},
		}}
	}

	cases := []struct {
		name     string
		ctx      context.Context
		token    string
		kid      string
		key      *rsa.PublicKey
		expected string
	}{
		{
			name:     "invalid signing method",
			token:    auth.BearerPrefix + invalidMethodToken,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "malformed token",
			token:    validToken,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "no iss claims",
			token:    auth.BearerPrefix + noIssClaimsToken,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "kid not found",
			token:    auth.BearerPrefix + noKidHeaderToken,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "expired claims",
			token:    auth.BearerPrefix + expiredClaimsToken,
			kid:      kid,
			key:      expiredClaimsTokenPublicKey,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "not before claims",
			token:    auth.BearerPrefix + notBeforeClaimsToken,
			kid:      kid,
			key:      notBeforeClaimsTokenPublicKey,
			expected: "authorization failed: unauthorized",
		},
		{
			name:     "invalid key",
			token:    auth.BearerPrefix + validToken,
			kid:      kid,
			key:      expiredClaimsTokenPublicKey,
			expected: "authorization failed: unauthorized",
		},
		{
			name:  "valid token", // no error is expected because authz is not tested here - the provided input to `Authencicate` isn't enough to pass authz
			token: auth.BearerPrefix + validToken,
			kid:   kid,
			key:   validTokenPublicKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockProvider(t)
			auth, err := auth.NewOidcAuthenticator(mock, nil)
			assert.NoError(t, err)

			if tc.kid != "" {
				mock.On("GetSigningKey", tc.kid).Return(tc.key, nil)
			}

			err = auth.Authenticate(context.Background(), constructInput(tc.token))
			if tc.expected != "" {
				assert.ErrorContains(t, err, tc.expected)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
