// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// kcClient is a minimal representation of a Keycloak client object we care about.
type kcClient struct {
	ID         string            `json:"id"`
	ClientID   string            `json:"clientId"`
	Attributes map[string]string `json:"attributes"`
}

// EnforceClientAccessTokenTTL ensures the given Keycloak client has the desired access token lifespan (in seconds).
// It runs a single idempotent reconciliation: only performs an update if the value differs.
// Any error is logged and suppressed (never returns an error) to avoid blocking service startup.
func EnforceClientAccessTokenTTL(ctx context.Context, oidcURL string, realm string, clientID string, desired time.Duration, adminToken string, logger *slog.Logger) {
	if clientID == "" {
		logger.Warn("skip TTL enforcement: empty clientID")
		return
	}
	if desired <= 0 {
		logger.Warn("skip TTL enforcement: non-positive desired duration", "desired", desired)
		return
	}
	base, derivedRealm, err := deriveBaseAndRealm(oidcURL)
	if err != nil {
		logger.Warn("cannot derive base/realm from OIDC URL", "error", err, "oidc", oidcURL)
		return
	}
	if realm == "" {
		realm = derivedRealm
	}

	secs := int64(desired.Seconds())
	desiredStr := fmt.Sprintf("%d", secs)

	// Step 1: lookup UUID
	uuid, err := kcLookupClientUUID(ctx, base, realm, clientID, adminToken)
	if err != nil {
		logger.Warn("client TTL enforcement failed (lookup)", "error", err, "client", clientID)
		return
	}
	// Step 2: get full client
	cl, err := kcGetClient(ctx, base, realm, uuid, adminToken)
	if err != nil {
		logger.Warn("client TTL enforcement failed (get)", "error", err, "client", clientID)
		return
	}
	if cl.Attributes == nil {
		cl.Attributes = map[string]string{}
	}
	current := cl.Attributes["access.token.lifespan"]
	if current == desiredStr {
		logger.Info("keycloak client TTL already compliant", "client", clientID, "ttl_seconds", desiredStr)
		return
	}
	cl.Attributes["access.token.lifespan"] = desiredStr
	if err := kcUpdateClient(ctx, base, realm, uuid, adminToken, cl); err != nil {
		logger.Warn("client TTL enforcement failed (update)", "error", err, "client", clientID)
		return
	}
	logger.Info("keycloak client TTL updated", "client", clientID, "old", current, "new", desiredStr)
}

// deriveBaseAndRealm extracts base host (scheme://host[:port]) and realm name from a standard Keycloak OIDC issuer URL.
func deriveBaseAndRealm(oidc string) (string, string, error) {
	if oidc == "" {
		return "", "", errors.New("empty OIDC URL")
	}
	u, err := neturl.Parse(oidc)
	if err != nil {
		return "", "", fmt.Errorf("parse oidc url: %w", err)
	}
	// Path typically /realms/<realm>
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var realm string
	for i := 0; i < len(parts); i++ {
		if parts[i] == "realms" && i+1 < len(parts) {
			realm = parts[i+1]
			break
		}
	}
	if realm == "" {
		return "", "", fmt.Errorf("realm segment not found in path '%s'", u.Path)
	}
	base := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	return base, realm, nil
}

func kcLookupClientUUID(ctx context.Context, base, realm, clientID, token string) (string, error) {
	url := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", base, realm, clientID)
	var list []kcClient
	if err := kcDoJSON(ctx, http.MethodGet, url, token, nil, &list); err != nil {
		return "", err
	}
	if len(list) != 1 {
		return "", fmt.Errorf("expected exactly 1 client for id '%s', got %d", clientID, len(list))
	}
	if list[0].ID == "" {
		return "", errors.New("client UUID empty in lookup response")
	}
	return list[0].ID, nil
}

func kcGetClient(ctx context.Context, base, realm, uuid, token string) (*kcClient, error) {
	url := fmt.Sprintf("%s/admin/realms/%s/clients/%s", base, realm, uuid)
	var cl kcClient
	if err := kcDoJSON(ctx, http.MethodGet, url, token, nil, &cl); err != nil {
		return nil, err
	}
	return &cl, nil
}

func kcUpdateClient(ctx context.Context, base, realm, uuid, token string, cl *kcClient) error {
	url := fmt.Sprintf("%s/admin/realms/%s/clients/%s", base, realm, uuid)
	return kcDoJSON(ctx, http.MethodPut, url, token, cl, nil)
}

func kcDoJSON(ctx context.Context, method, url, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s -> status %d body: %s", method, url, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
