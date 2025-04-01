// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/golang-jwt/jwt/v5"

	opa "github.com/open-edge-platform/orch-library/go/pkg/openpolicyagent"
)

var (
	// validSigningMethods is a list of all valid signing methods to verify the jwt
	validSigningMethods = []string{"PS512"}
)

// NewOidcAuthenticator returns a new OIDC Authenticator
func NewOidcAuthenticator(provider provider, opa opa.ClientWithResponsesInterface) (*oidcAuthenticator, error) {
	return &oidcAuthenticator{
		provider: provider,
		opa:      opa,
	}, nil
}

// Authenticate is used as AuthenticationFunc in the openapi3filter
func (auth oidcAuthenticator) Authenticate(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
	if input == nil {
		slog.Error("authentication func missing input")
		return errors.New("authorization failed: unauthorized")
	}

	token, err := auth.authn(input.RequestValidationInput.Request)
	if err != nil {
		return newAuthError(input, fmt.Errorf("authn: %w", err))
	}

	if err := auth.authz(input.RequestValidationInput.Request, token); err != nil {
		return newAuthError(input, fmt.Errorf("authz: %w", err))
	}

	return nil
}

// authn authenticates the token using the OIDC server
func (auth oidcAuthenticator) authn(req *http.Request) (*jwt.Token, error) {
	// extract bearer token from request header
	bearerToken := getAuthHeader(req)
	if bearerToken == "" {
		return nil, errors.New("missing authentication token")
	}

	// remove 'Bearer ' prefix from token
	rawToken, ok := strings.CutPrefix(bearerToken, "Bearer ")
	if !ok || rawToken == "" {
		return nil, errors.New("malformed token")
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, auth.getKeyFunc(), jwt.WithValidMethods(validSigningMethods))
	if err != nil {
		return nil, fmt.Errorf("failed to parse token with claims: %w", err)
	}

	return token, nil
}

// authz authorizes the token based on the claims
func (auth oidcAuthenticator) authz(req *http.Request, token *jwt.Token) error {
	if auth.opa == nil {
		slog.Warn("opa is not enabled, skipping authorization")
		return nil
	}

	// extract active project id from request header
	projectId := getProjectHeader(req)
	if projectId == "" {
		return errors.New("missing active project id")
	}

	// extract roles from token
	roles, err := extractRolesFromToken(token)
	if err != nil {
		return fmt.Errorf("failed to extract roles from token: %w", err)
	}

	// evaluate policy
	return evaluatePolicy(req.Context(), auth.opa, roles, req.Method, req.URL.Path, projectId)
}

// getKeyFunc returns a jwt.Keyfunc that dynamically selects the key based on the issuer
// this key is used by jwt.ParseWithClaims to verify the signature of the token
func (auth oidcAuthenticator) getKeyFunc() jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return nil, errors.New("invalid token claims")
		}

		if _, ok = claims["iss"].(string); !ok {
			return nil, errors.New("issuer claim 'iss' not found")
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("token header 'kid' not found")
		}

		return auth.provider.GetSigningKey(kid)
	}
}

// NewNoopAuthenticator returns a new no-op Authenticator
func NewNoopAuthenticator() *noopAuthenticator {
	return &noopAuthenticator{}
}

// Authenticate is a no-op authenticator
func (auth noopAuthenticator) Authenticate(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
	return openapi3filter.NoopAuthenticationFunc(ctx, input)
}
