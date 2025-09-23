// SPDX-FileCopyrightText: (C) 2023 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	vaultK8STokenFile  = `/var/run/secrets/kubernetes.io/serviceaccount/token` // #nosec G101
	vaultK8SLoginURL   = `/v1/auth/kubernetes/login`
	vaultSecretBaseURL = `/v1/secret/data/` // #nosec
	m2mVaultClient     = "co-manager-m2m-client-secret"
	VaultServer        = "http://vault.orch-platform.svc.cluster.local:8200"
	ServiceAccount     = "cluster-manager"
)

type VaultAuth interface {
	GetClientCredentials(ctx context.Context) (string, string, error)
}

type vaultAuth struct {
	vaultServer    string
	serviceAccount string
	httpClient     *http.Client
	vaultToken     string
	mu             sync.Mutex
}

func NewVaultAuth(vaultServer string, serviceAccount string) (VaultAuth, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &vaultAuth{
		httpClient:     client,
		vaultServer:    vaultServer,
		serviceAccount: serviceAccount,
	}, nil
}

func (v *vaultAuth) httpsVaultURL(path string) string {
	return v.vaultServer + path
}

func (v *vaultAuth) getVaultToken(ctx context.Context) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.vaultToken != "" {
		return v.vaultToken, nil
	}

	tokenData, err := os.ReadFile(vaultK8STokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read Kubernetes token file: %w", err)
	}

	loginReq := struct {
		JWT  string `json:"jwt"`
		Role string `json:"role"`
	}{
		JWT:  string(tokenData),
		Role: v.serviceAccount,
	}

	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.httpsVaultURL(vaultK8SLoginURL), bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault login request failed with status code %d", resp.StatusCode)
	}

	var loginResp struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("failed to decode login response: %w", err)
	}

	if loginResp.Auth.ClientToken == "" {
		return "", fmt.Errorf("vault login response did not contain a client token")
	}

	v.vaultToken = loginResp.Auth.ClientToken
	return v.vaultToken, nil
}

func (v *vaultAuth) GetClientCredentials(ctx context.Context) (string, string, error) {
	vaultToken, err := v.getVaultToken(ctx)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.httpsVaultURL(vaultSecretBaseURL+m2mVaultClient), nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request to fetch client credentials: %w", err)
	}
	req.Header.Add("X-Vault-Token", vaultToken)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch client credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to fetch client credentials, status code %d", resp.StatusCode)
	}

	var secretResp struct {
		Data struct {
			Data struct {
				ClientID     string `json:"client_id"`
				ClientSecret string `json:"client_secret"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&secretResp); err != nil {
		return "", "", fmt.Errorf("failed to decode client credentials response: %w", err)
	}

	return secretResp.Data.Data.ClientID, secretResp.Data.Data.ClientSecret, nil
}
