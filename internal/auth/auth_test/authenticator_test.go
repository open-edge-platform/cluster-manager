// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestNewOidcAuthenticator(t *testing.T) {
	authenticator, err := auth.NewOidcAuthenticator(nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, authenticator)
	assert.Error(t, authenticator.Authenticate(context.Background(), nil))

	// TODO: test with provider and rego rules
}

func TestAuthenticate(t *testing.T) {
	cases := []struct {
		name     string
		input    *openapi3filter.AuthenticationInput
		expected string
	}{
		{
			name:     "missing input",
			expected: "authorization failed: unauthorized",
		},
		{
			name: "missing authentication token",
			input: &openapi3filter.AuthenticationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request: &http.Request{Header: http.Header{}},
			}},
			expected: "authorization failed: unauthorized",
		},
		{
			name: "malformed token",
			input: &openapi3filter.AuthenticationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request: &http.Request{
					Header: http.Header{auth.AuthorizationHeaderKey: {"token"}},
				},
			}},
			expected: "authorization failed: unauthorized",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auth, err := auth.NewOidcAuthenticator(nil, nil)
			assert.NoError(t, err)

			err = auth.Authenticate(context.Background(), tc.input)
			if tc.expected != "" {
				assert.ErrorContains(t, err, tc.expected)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNoopAuthenticate(t *testing.T) {
	auth := auth.NewNoopAuthenticator()

	assert.NoError(t, auth.Authenticate(context.Background(), nil))
	assert.NoError(t, auth.Authenticate(context.Background(), &openapi3filter.AuthenticationInput{}))
}
