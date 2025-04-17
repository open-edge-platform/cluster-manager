// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// JwtToken retrieves a new token from Keycloak
func JwtToken(keycloakURL, accessToken string) (*TokenResponse, error) {
	clientId, username, _, _ := ExtractClaims(accessToken)
	password := os.Getenv("PASSWORD")

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", clientId)
	data.Set("username", username)
	data.Set("password", password)

	tokenURL := fmt.Sprintf("%s/protocol/openid-connect/token", keycloakURL)
	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to refresh token, status code: %d", resp.StatusCode)
	}
	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, err
	}

	return &tokenResponse, nil
}

// ExtractClaims extracts claims from a JWT token
func ExtractClaims(tokenString string) (string, string, time.Time, error) {
	// Parse the token without verifying the signature to extract the claims
	// but since the token has passed the auth check, we can trust the claims - Confirm
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", time.Time{}, fmt.Errorf("failed to extract claims")
	}

	azp, _ := claims["azp"].(string) // authorized party
	preferredUsername, _ := claims["preferred_username"].(string)

	exp, ok := claims["exp"].(float64) // expire time
	if !ok {
		return azp, preferredUsername, time.Time{}, fmt.Errorf("invalid or missing exp claim")
	}

	expirationTime := time.Unix(int64(exp), 0) // convert to time.Time

	return azp, preferredUsername, expirationTime, nil
}
