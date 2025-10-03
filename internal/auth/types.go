// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"

	"github.com/lestrrat-go/jwx/v2/jwk"
	opa "github.com/open-edge-platform/orch-library/go/pkg/openpolicyagent"
)

const (
	OidcUrlEnvVar     = "OIDC_SERVER_URL"
	KeycloakUrlEnvVar = "KEYCLOAK_URL"
	OpaEnabledEnvVar  = "OPA_ENABLED"
	OpaPortEnvVar     = "OPA_PORT"
)

// oidcAuthenticator is an implementation of the Authenticator interface that uses OIDC for authentication
type oidcAuthenticator struct {
	provider provider
	opa      opa.ClientWithResponsesInterface
}

// noopAuthenticator is an implementation of the Authenticator interface that does nothing
type noopAuthenticator struct{}

// provider is an interface that can be used with an Authenticator to verify tokens
type provider interface {
	GetSigningKey(kid string) (interface{}, error)
}

// oidcProvider is an implementation of the Provider interface that uses OIDC for token verification
type oidcProvider struct {
	endpoint string
	client   *http.Client
	config   *oidcProviderConfig
	jwks     *jwk.Cache
}

// oidcProviderConfig represents the OIDC provider's configuration
type oidcProviderConfig struct {
	Issuer        string   `json:"issuer"`
	AuthUrl       string   `json:"authorization_endpoint"`
	TokenUrl      string   `json:"token_endpoint"`
	DeviceAuthUrl string   `json:"device_authorization_endpoint"`
	JwksUrl       string   `json:"jwks_uri"`
	UserInfoUrl   string   `json:"userinfo_endpoint"`
	Algorithms    []string `json:"id_token_signing_alg_values_supported"`
}
