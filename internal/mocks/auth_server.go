// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

func RunAuthServer() {
	mockCmd := flag.NewFlagSet("mock-server", flag.ExitOnError)
	mockType := mockCmd.String("type", "", "Type of mock server: keycloak or vault")
	port := mockCmd.Int("port", 8080, "Port to listen on")

	if err := mockCmd.Parse(os.Args[2:]); err != nil {
		slog.Error("failed to parse flags", "error", err)
		os.Exit(1)
	}

	if *mockType == "keycloak" {
		runKeycloakMock(*port)
	} else if *mockType == "vault" {
		runVaultMock(*port)
	} else {
		slog.Error("Invalid mock type. Use -type=keycloak or -type=vault")
		os.Exit(1)
	}
}

// Keycloak Mock
var (
	mockKeycloakPrivateKey *rsa.PrivateKey
	mockKeycloakPublicKey  *rsa.PublicKey
	mockKeycloakKID        = "mock-key-id"
)

func runKeycloakMock(port int) {
	var err error
	mockKeycloakPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("failed to generate RSA key", "error", err)
		os.Exit(1)
	}
	mockKeycloakPublicKey = &mockKeycloakPrivateKey.PublicKey

	http.HandleFunc("/realms/master/protocol/openid-connect/certs", handleJWKS)
	http.HandleFunc("/realms/master/protocol/openid-connect/token", handleToken)
	http.HandleFunc("/realms/master/.well-known/openid-configuration", handleOIDCConfig)
	http.HandleFunc("/admin/realms/master/clients", handleClients)
	http.HandleFunc("/admin/realms/master/clients/", handleClientDetail)
	http.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("Starting Keycloak mock", "addr", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func handleOIDCConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"issuer":                 "http://platform-keycloak.orch-platform.svc/realms/master",
		"authorization_endpoint": "http://platform-keycloak.orch-platform.svc/realms/master/protocol/openid-connect/auth",
		"token_endpoint":         "http://platform-keycloak.orch-platform.svc/realms/master/protocol/openid-connect/token",
		"jwks_uri":               "http://platform-keycloak.orch-platform.svc/realms/master/protocol/openid-connect/certs",
		"response_types_supported": []string{
			"code", "none", "id_token", "token", "id_token token", "code id_token", "code token", "code id_token token",
		},
		"subject_types_supported":               []string{"public", "pairwise"},
		"id_token_signing_alg_values_supported": []string{"PS512", "RS256"},
	}
	json.NewEncoder(w).Encode(resp)
}

func handleClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`[{"id":"mock-client-uuid","clientId":"test-client","enabled":true,"attributes":{"access.token.lifespan":"10800"}}]`))
}

func handleClientDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		w.Write([]byte(`{"id":"mock-client-uuid","clientId":"test-client","enabled":true,"attributes":{"access.token.lifespan":"10800"}}`))
	} else if r.Method == http.MethodPut {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleJWKS(w http.ResponseWriter, r *http.Request) {
	key, err := jwk.FromRaw(mockKeycloakPublicKey)
	if err != nil {
		http.Error(w, "failed to create JWK", http.StatusInternalServerError)
		return
	}

	key.Set(jwk.KeyIDKey, mockKeycloakKID)
	key.Set(jwk.AlgorithmKey, "PS512")
	key.Set(jwk.KeyUsageKey, "sig")

	set := jwk.NewSet()
	set.AddKey(key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	rolesStr := r.FormValue("roles")
	var roles []string
	if rolesStr != "" {
		roles = strings.Split(rolesStr, ",")
	} else {
		roles = []string{"admin"}
	}

	// Simple mock: always return a valid token
	token := jwt.NewWithClaims(jwt.SigningMethodPS512, jwt.MapClaims{
		"iss": "http://platform-keycloak.orch-platform.svc/realms/master",
		"sub": "mock-user",
		"aud": "cluster-manager",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"realm_access": map[string]interface{}{
			"roles": roles,
		},
	})

	token.Header["kid"] = mockKeycloakKID

	tokenString, err := token.SignedString(mockKeycloakPrivateKey)
	if err != nil {
		http.Error(w, "Failed to sign token", http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"access_token":       tokenString,
		"expires_in":         3600,
		"refresh_expires_in": 3600,
		"token_type":         "Bearer",
		"not-before-policy":  0,
		"scope":              "profile email",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Vault Mock
func runVaultMock(port int) {
	http.HandleFunc("/v1/auth/kubernetes/login", handleVaultLogin)
	http.HandleFunc("/v1/secret/data/co-manager-m2m-client-secret", handleVaultSecret)
	http.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("Starting Vault mock", "addr", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func handleVaultLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]interface{}{
		"auth": map[string]interface{}{
			"client_token":   "mock-vault-token",
			"lease_duration": 3600,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleVaultSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check for X-Vault-Token header
	if r.Header.Get("X-Vault-Token") == "" {
		http.Error(w, "Missing X-Vault-Token header", http.StatusForbidden)
		return
	}

	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"data": map[string]interface{}{
				"client_id":     "co-manager-m2m-client",
				"client_secret": "test-secret",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
