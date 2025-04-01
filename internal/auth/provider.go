// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

const (
	// oidConfigPath is the open id provider's well-known configuration endpoint
	oidConfigPath = ".well-known/openid-configuration"
)

// NewOidcProvider creates a new OIDC provider using it's well-known configuration
func NewOidcProvider(endpoint string) (*oidcProvider, error) {
	if endpoint == "" {
		return nil, errors.New("failed to create oidc provider: endpoint is empty")
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: false}}}

	// fetch the well known config from the oidc provider
	config, err := getWellKnownConfig(client, endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting keycloak well known config: %w", err)
	}

	// create and refresh jwks cache
	jwks := jwk.NewCache(context.Background())
	if err := jwks.Register(config.JwksUrl, jwk.WithHTTPClient(client)); err != nil {
		return nil, fmt.Errorf("error registering jwks cache: %w", err)
	}
	if _, err := jwks.Refresh(context.Background(), config.JwksUrl); err != nil {
		return nil, fmt.Errorf("error refreshing jwks cache: %w", err)
	}

	return &oidcProvider{
		endpoint: endpoint,
		client:   client,
		config:   config,
		jwks:     jwks,
	}, nil
}

// GetSigningKey gets the public signing key from the jwks cache
// If the key is not found in the cache, it will refresh the cache and try again
func (p *oidcProvider) GetSigningKey(kid string) (interface{}, error) {
	// get jwks from cache
	set, err := p.jwks.Get(context.Background(), p.config.JwksUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to get jwks from cache: %w", err)
	}

	// get the key that was used to sign the jwt
	key, found := set.LookupKeyID(kid)
	if !found {
		// refresh the cache and try again
		set, err = p.jwks.Refresh(context.Background(), p.config.JwksUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to get jwks from provider: %w", err)
		}

		key, found = set.LookupKeyID(kid)
		if !found {
			return nil, errors.New("key not found")
		}
	}

	var rawKey interface{}
	if err = key.Raw(&rawKey); err != nil {
		return nil, fmt.Errorf("failed to create public key: %w", err)
	}

	return rawKey, nil
}
