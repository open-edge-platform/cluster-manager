// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func newToken(method jwt.SigningMethod, header map[string]interface{}, claims jwt.Claims) *jwt.Token {
	token := jwt.NewWithClaims(method, claims)
	for k, v := range header {
		token.Header[k] = v
	}

	return token
}

func signToken(t *testing.T, token *jwt.Token) (string, *rsa.PublicKey) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	signed, err := token.SignedString(key)
	assert.NoError(t, err)

	return signed, &key.PublicKey
}
