// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// kcClient is a minimal keycloak admin client for per-client access token ttl management
type kcClient struct {
	ID         string            `json:"id"`
	ClientID   string            `json:"clientId"`
	Attributes map[string]string `json:"attributes"`
}

// EnforceClientAccessTokenTTL sets the client's access token lifespan if different from desired value.
// Returns true on success (including already correct), false on failure.
func EnforceClientAccessTokenTTL(ctx context.Context, oidcURL string, realm string, clientID string, desired time.Duration, adminToken string) bool {
	if clientID == "" || desired <= 0 {
		slog.Debug("skip TTL enforcement: invalid input")
		return false
	}

	base, derivedRealm, err := deriveBaseAndRealm(oidcURL)
	if err != nil {
		slog.Warn("cannot derive keycloak endpoint", "error", err)
		return false
	}
	if realm == "" {
		realm = derivedRealm
	}

	cl, uuid, err := kcGetClientByClientID(ctx, base, realm, clientID, adminToken)
	if err != nil {
		slog.Warn("failed to get client for enforcement", "error", err)
		return false
	}

	desiredStr := fmt.Sprintf("%d", int64(desired.Seconds()))
	current := cl.Attributes["access.token.lifespan"]
	if current == desiredStr {
		slog.Debug("client TTL already correct in keycloak")
		return true
	}
	cl.Attributes["access.token.lifespan"] = desiredStr
	if err := kcUpdateClient(ctx, base, realm, uuid, adminToken, cl); err != nil {
		slog.Error("failed to update access token ttl", "error", err)
		return false
	}
	slog.Info("client TTL updated", "previous", current, "new", desiredStr)
	return true
}

// ClearClientAccessTokenTTL removes per-client token lifespan override to inherit realm default.
// Returns true if override absent or successfully cleared; false on failure.
func ClearClientAccessTokenTTL(ctx context.Context, oidcURL string, realm string, clientID string, adminToken string) bool {
	if clientID == "" {
		slog.Debug("no clientID setup")
		return false
	}
	base, derivedRealm, err := deriveBaseAndRealm(oidcURL)
	if err != nil {
		slog.Warn("cannot derive keycloak endpoint for clear", "error", err)
		return false
	}
	if realm == "" {
		realm = derivedRealm
	}
	cl, uuid, err := kcGetClientByClientID(ctx, base, realm, clientID, adminToken)
	if err != nil {
		slog.Warn("failed to get client for ttl clear", "error", err)
		return false
	}

	old, ok := cl.Attributes["access.token.lifespan"]
	if !ok { // already default
		slog.Debug("no TTL override to clear")
		return true
	}

	delete(cl.Attributes, "access.token.lifespan")
	if err := kcUpdateClient(ctx, base, realm, uuid, adminToken, cl); err != nil {
		slog.Error("failed to clear client TTL override (delete attribute)", "error", err)
		return false
	}

	// verify removal
	clVerify, err := kcGetClient(ctx, base, realm, uuid, adminToken)
	if err != nil {
		slog.Warn("failed to re-fetch client after clear", "error", err)
		return false
	}

	if clVerify.Attributes != nil {
		if v2, still := clVerify.Attributes["access.token.lifespan"]; still {
			// fallback: explicit empty string
			clVerify.Attributes["access.token.lifespan"] = ""
			if err2 := kcUpdateClient(ctx, base, realm, uuid, adminToken, clVerify); err2 != nil {
				slog.Warn("fallback empty-string clear failed", "error", err2, "current_value", v2)
				return false
			}
			if clFinal, err3 := kcGetClient(ctx, base, realm, uuid, adminToken); err3 == nil && clFinal.Attributes != nil {
				if vf, still2 := clFinal.Attributes["access.token.lifespan"]; still2 && vf != "" {
					slog.Warn("TTL override still present after deletion + empty-string attempts", "value", vf)
					return false
				}
			} else if err3 != nil {
				slog.Warn("failed final fetch after fallback clear", "error", err3)
				return false
			}
		}
	}

	slog.Info("keycloak client TTL override cleared", "previous", old)
	return true
}

// deriveBaseAndRealm extracts base host (scheme://host[:port]) and realm name from a standard keycloak OIDC issuer URL
func deriveBaseAndRealm(oidc string) (string, string, error) {
	if oidc == "" {
		return "", "", errors.New("empty OIDC URL")
	}

	u, err := url.Parse(oidc)
	if err != nil {
		return "", "", fmt.Errorf("parse oidc url: %w", err)
	}

	// find realm in path: /realms/<realm-name>
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var realm string
	for i, part := range parts {
		if part == "realms" && i+1 < len(parts) {
			realm = parts[i+1]
			break
		}
	}

	if realm == "" {
		return "", "", fmt.Errorf("realm segment not found in path '%s'", u.Path)
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), realm, nil
}

// kcGetClientByClientID resolves the client UUID from a clientID then fetches the full client representation
func kcGetClientByClientID(ctx context.Context, base, realm, clientID, token string) (*kcClient, string, error) {
	uuid, err := kcLookupClientUUID(ctx, base, realm, clientID, token)
	if err != nil {
		return nil, "", err
	}

	cl, err := kcGetClient(ctx, base, realm, uuid, token)
	if err != nil {
		return nil, "", err
	}
	return cl, uuid, nil
}

func kcLookupClientUUID(ctx context.Context, base, realm, clientID, token string) (string, error) {
	if clientID == "" {
		return "", errors.New("empty clientID")
	}
	// URL encode clientID in case of special characters
	escapedId := url.QueryEscape(clientID)
	reqURL := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", base, realm, escapedId)

	var clients []kcClient
	if err := kcDoJSON(ctx, http.MethodGet, reqURL, token, nil, &clients); err != nil {
		return "", err
	}

	switch len(clients) {
	case 0:
		return "", fmt.Errorf("client not found: %s", clientID)
	case 1:
		if clients[0].ID == "" {
			return "", errors.New("client UUID is empty")
		}
		return clients[0].ID, nil
	default:
		return "", fmt.Errorf("expected exactly 1 client, but got %d", len(clients))
	}
}

func kcGetClient(ctx context.Context, base, realm, uuid, token string) (*kcClient, error) {
	reqURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s", base, realm, uuid)
	var cl kcClient // avoid returning nil

	if err := kcDoJSON(ctx, http.MethodGet, reqURL, token, nil, &cl); err != nil {
		return nil, err
	}

	// ensure Attributes map is non-nil for caller (avoid repetitive nil guards)
	if cl.Attributes == nil {
		cl.Attributes = map[string]string{}
	}

	return &cl, nil
}

func kcUpdateClient(ctx context.Context, base, realm, uuid, token string, cl *kcClient) error {
	reqURL := fmt.Sprintf("%s/admin/realms/%s/clients/%s", base, realm, uuid)

	return kcDoJSON(ctx, http.MethodPut, reqURL, token, cl, nil)
}

func kcDoJSON(ctx context.Context, method, urlStr, token string, body any, out any) error {
	var reqBody io.ReadCloser = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reqBody = io.NopCloser(bytes.NewReader(b))
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// reuse shared client (with already set timeout)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// drain so connection can be reused; best-effort (limit 8KB)
		if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10)); err != nil {
			slog.Debug("failed to drain error response body", "error", err)
		}
		return fmt.Errorf("request failed: %s %s status=%d", method, urlStr, resp.StatusCode)
	}

	// caller is not expecting a JSON response (out == nil). Drain (limit 8KB) so the connection can be reused
	if out == nil {
		if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10)); err != nil {
			slog.Debug("failed to drain success response body", "error", err)
		}
		return nil
	}

	if resp.ContentLength == 0 || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
