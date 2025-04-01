// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/open-edge-platform/cluster-manager/internal/auth"
	opa "github.com/open-edge-platform/orch-library/go/pkg/openpolicyagent"
)

func TestAuthZ(t *testing.T) {
	mockController := gomock.NewController(gomock.TestReporter(t))

	kid := "test-key"

	validMethod := jwt.SigningMethodPS512

	validClaims := jwt.MapClaims{
		"iss": "not-empty",
		"realm_access": map[string]interface{}{
			"roles": []string{"admin"},
		},
	}
	noRolesClaims := jwt.MapClaims{"iss": "not-empty"}

	validHeader := map[string]interface{}{"kid": kid}

	validToken, validTokenPublicKey := signToken(t, newToken(validMethod, validHeader, validClaims))
	noRolesToken, noRolesTokenPublicKey := signToken(t, newToken(validMethod, validHeader, noRolesClaims))

	emptyResult := opa.OpaResponse_Result{}
	trueResult := opa.OpaResponse_Result{}
	trueResult.FromOpaResponseResult1(true)
	falseResult := opa.OpaResponse_Result{}
	falseResult.FromOpaResponseResult1(false)

	constructInput := func(method, path, token, projectId string) *openapi3filter.AuthenticationInput {
		return &openapi3filter.AuthenticationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: &http.Request{
				Method: method,
				URL:    &url.URL{Path: path},
				Header: http.Header{
					auth.AuthorizationHeaderKey:   {auth.BearerPrefix + token},
					auth.ActiveProjectIdHeaderKey: {projectId},
				},
			},
		}}
	}

	cases := []struct {
		name         string
		ctx          context.Context
		method       string
		path         string
		token        string
		key          *rsa.PublicKey
		projectId    string
		expected     string
		mockedResult *opa.OpaResponse_Result
		mockedError  error
	}{
		{
			name:  "opa disabled",
			token: validToken,
			key:   validTokenPublicKey,
		},
		{
			name:         "no active project id",
			token:        validToken,
			key:          validTokenPublicKey,
			expected:     "authorization failed: unauthorized",
			mockedResult: &emptyResult,
		},
		{
			name:         "no realm access roles",
			token:        noRolesToken,
			key:          noRolesTokenPublicKey,
			projectId:    "test-project",
			expected:     "authorization failed: unauthorized",
			mockedResult: &emptyResult,
		},
		{
			name:         "fail to evaluate policy",
			token:        validToken,
			key:          validTokenPublicKey,
			projectId:    "test-project",
			expected:     "authorization failed: unauthorized",
			mockedResult: &emptyResult,
			mockedError:  errors.New("fake error"),
		},
		{
			name:         "fail to parse result",
			token:        validToken,
			key:          validTokenPublicKey,
			projectId:    "test-project",
			expected:     "authorization failed: unauthorized",
			mockedResult: &emptyResult,
		},
		{
			name:         "unauthorized",
			method:       "GET",
			path:         "/v2/clusters",
			token:        validToken,
			key:          validTokenPublicKey,
			projectId:    "test-project",
			expected:     "authorization failed: unauthorized",
			mockedResult: &falseResult,
		},
		{
			name:         "authorized",
			method:       "GET",
			path:         "/v2/clusters",
			token:        validToken,
			key:          validTokenPublicKey,
			projectId:    "test-project",
			mockedResult: &trueResult,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockProvider := NewMockProvider(t)
			mockOpa := opa.NewMockClientWithResponsesInterface(mockController)
			authenticator, err := auth.NewOidcAuthenticator(mockProvider, mockOpa)
			assert.NoError(t, err)

			input := constructInput(tc.method, tc.path, tc.token, tc.projectId)

			mockProvider.On("GetSigningKey", kid).Return(tc.key, nil)

			if tc.mockedResult == nil {
				authenticator, err = auth.NewOidcAuthenticator(mockProvider, nil)
			} else {
				mockOpa.EXPECT().PostV1DataPackageRuleWithBodyWithResponse(
					input.RequestValidationInput.Request.Context(), "authz", "allow", gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&opa.PostV1DataPackageRuleResponse{JSON200: &opa.OpaResponse{Result: *tc.mockedResult}}, tc.mockedError).AnyTimes()
			}

			err = authenticator.Authenticate(context.Background(), input)
			if tc.expected != "" {
				assert.ErrorContains(t, err, tc.expected)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
