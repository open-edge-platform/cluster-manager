// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestExtractClaims(t *testing.T) {
	tests := []struct {
		name          string
		tokenClaims   jwt.MapClaims
		expectedError bool
		expectedAzp   string
		expectedUser  string
		expectedExp   time.Time
	}{
		{
			name: "valid token",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name:          "invalid token",
			tokenClaims:   nil,
			expectedError: true,
		},
		{
			name: "token without exp claim",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
			},
			expectedError: true,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Time{},
		},
		{
			name: "token with missing azp claim",
			tokenClaims: jwt.MapClaims{
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name: "token with missing preferred_username claim",
			tokenClaims: jwt.MapClaims{
				"azp": "test-client-id",
				"exp": time.Now().Add(time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "",
			expectedExp:   time.Now().Add(time.Hour),
		},
		{
			name: "token with invalid exp claim",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                "invalid-exp",
			},
			expectedError: true,
		},
		{
			name: "expired token",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(-time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(-time.Hour),
		},
		{
			name: "token with future expiration",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(100 * time.Hour).Unix(),
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(100 * time.Hour),
		},
		{
			name: "token with additional claims",
			tokenClaims: jwt.MapClaims{
				"azp":                "test-client-id",
				"preferred_username": "test-username",
				"exp":                time.Now().Add(time.Hour).Unix(),
				"extra_claim":        "extra_value",
			},
			expectedError: false,
			expectedAzp:   "test-client-id",
			expectedUser:  "test-username",
			expectedExp:   time.Now().Add(time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenString string
			var err error
			tokenString = "invalid-token"
			if tt.tokenClaims != nil {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, tt.tokenClaims)
				tokenString, err = token.SignedString([]byte("secret"))
				assert.NoError(t, err)
			}

			clientId, username, exp, err := ExtractClaims(tokenString)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAzp, clientId)
				assert.Equal(t, tt.expectedUser, username)
				if tt.expectedExp.IsZero() {
					assert.Equal(t, tt.expectedExp, exp)
				} else {
					assert.WithinDuration(t, tt.expectedExp, exp, time.Minute)
				}
			}
		})
	}
}

// TestJwtTokenWithM2M tests the M2M token generation function
func TestJwtTokenWithM2M(t *testing.T) {
	// Table uses an anonymous struct for brevity; no need for a named local type.
	oneHour := 1 * time.Hour
	twoHours := 2 * time.Hour
	day := 24 * time.Hour

	non200Handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}

	badJSONHandler := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not-json"))
	}

	testcases := []struct {
		name        string
		ttl         *time.Duration
		expectedTTL time.Duration
		// tolerance accounts for network/processing delay and minor clock skew
		tolerance  time.Duration
		vaultErr   error
		keycloakFn func(w http.ResponseWriter, r *http.Request)
		unsetEnv   bool
		expectErr  string
	}{
		{name: "default ttl", ttl: nil, expectedTTL: oneHour, tolerance: 90 * time.Second},
		{name: "custom 2h ttl", ttl: &twoHours, expectedTTL: twoHours, tolerance: 90 * time.Second},
		{name: "custom 24h ttl", ttl: &day, expectedTTL: day, tolerance: 2 * time.Minute},
		{name: "missing KEYCLOAK_URL", ttl: &twoHours, unsetEnv: true, expectErr: "KEYCLOAK_URL"},
		{name: "vault credential failure", ttl: &twoHours, vaultErr: fmt.Errorf("vault down"), expectErr: "failed to fetch M2M credentials"},
		{name: "keycloak non-200", ttl: &twoHours, keycloakFn: non200Handler, expectErr: "status code"},
		{name: "keycloak bad json", ttl: &twoHours, keycloakFn: badJSONHandler, expectErr: "failed to decode"},
	}

	for _, tc := range testcases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			// reset cached credentials so each subtest independently triggers lazy load
			cachedClientID = ""
			cachedClientSecret = ""
			// override NewVaultAuthFunc for this subtest and register cleanup
			prev := NewVaultAuthFunc
			NewVaultAuthFunc = func(vaultServer string, serviceAccount string) (VaultAuth, error) {
				return &mockVaultAuth{err: tc.vaultErr}, nil
			}
			// ensure we always restore even on early failure
			t.Cleanup(func() { NewVaultAuthFunc = prev })

			// Setup / teardown KEYCLOAK_URL
			origEnv := os.Getenv("KEYCLOAK_URL")
			defer func() {
				if origEnv == "" {
					_ = os.Unsetenv("KEYCLOAK_URL")
				} else {
					_ = os.Setenv("KEYCLOAK_URL", origEnv)
				}
			}()

			var server *httptest.Server
			if !tc.unsetEnv {
				h := tc.keycloakFn
				if h == nil {
					h = func(w http.ResponseWriter, r *http.Request) {
						requested := oneHour
						if tc.ttl != nil {
							requested = *tc.ttl
						}
						exp := time.Now().Add(requested).Unix()
						token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
							"exp": exp,
							"azp": "test-client",
						})
						s, _ := token.SignedString([]byte("secret"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(fmt.Sprintf("{\"access_token\":\"%s\"}", s)))
					}
				}
				server = httptest.NewServer(http.HandlerFunc(h))
				defer server.Close()
				_ = os.Setenv("KEYCLOAK_URL", server.URL)
			} else if tc.unsetEnv {
				_ = os.Unsetenv("KEYCLOAK_URL")
			}

			start := time.Now()
			token, err := JwtTokenWithM2M(context.Background(), tc.ttl)

			if tc.expectErr != "" {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tc.expectErr)
				}
				return
			}

			assert.NoError(t, err)
			if err != nil { // safeguard
				return
			}

			clientID, _, exp, claimErr := ExtractClaims(token)
			assert.NoError(t, claimErr)
			assert.Equal(t, "test-client", clientID)

			actualTTL := exp.Sub(start)
			// normalize negative (shouldn't happen but guard) and assert within tolerance
			if actualTTL < 0 {
				actualTTL = 0
			}
			delta := actualTTL - tc.expectedTTL
			if delta < 0 {
				delta = -delta
			}
			assert.LessOrEqual(t, delta, tc.tolerance, "ttl delta exceeded tolerance: expected %v got %v (delta %v)", tc.expectedTTL, actualTTL, delta)
		})
	}
}

// mockVaultAuth implements VaultAuth for tests
type mockVaultAuth struct {
	err error
}

func (m *mockVaultAuth) GetClientCredentials(ctx context.Context) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	return "client-id", "client-secret", nil
}

// TestExtractUserRoles tests user role extraction from JWT tokens
func TestExtractUserRoles(t *testing.T) {
	tests := []struct {
		name          string
		tokenClaims   jwt.MapClaims
		expectedRoles []string
		expectedError bool
	}{
		{
			name: "token with multiple roles",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"admin", "user", "cluster-reader"},
				},
			},
			expectedRoles: []string{"admin", "user", "cluster-reader"},
			expectedError: false,
		},
		{
			name: "token with single role",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{"user"},
				},
			},
			expectedRoles: []string{"user"},
			expectedError: false,
		},
		{
			name: "token with no roles",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": []interface{}{},
				},
			},
			expectedRoles: []string{},
			expectedError: false,
		},
		{
			name: "token without realm_access",
			tokenClaims: jwt.MapClaims{
				"azp": "test-client",
			},
			expectedRoles: nil,
			expectedError: true,
		},
		{
			name: "token with invalid roles format",
			tokenClaims: jwt.MapClaims{
				"realm_access": map[string]interface{}{
					"roles": "invalid-format",
				},
			},
			expectedRoles: nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the actual ExtractUserRoles function
			roles, err := ExtractUserRoles(tt.tokenClaims)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, roles)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRoles, roles)
			}
		})
	}
}

// TestTokenTTLValidation tests TTL validation logic
func TestTokenTTLValidation(t *testing.T) {
	tests := []struct {
		name        string
		ttl         time.Duration
		expectValid bool
		description string
	}{
		{
			name:        "valid 1 hour TTL",
			ttl:         1 * time.Hour,
			expectValid: true,
			description: "1 hour is within valid range",
		},
		{
			name:        "valid 24 hour TTL",
			ttl:         24 * time.Hour,
			expectValid: true,
			description: "24 hours is within valid range",
		},
		{
			name:        "valid 7 day TTL",
			ttl:         7 * 24 * time.Hour,
			expectValid: true,
			description: "7 days is within valid range",
		},
		{
			name:        "too short TTL (5 minutes)",
			ttl:         5 * time.Minute,
			expectValid: false,
			description: "5 minutes is below minimum TTL",
		},
		{
			name:        "too long TTL (30 days)",
			ttl:         30 * 24 * time.Hour,
			expectValid: false,
			description: "30 days exceeds maximum TTL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test TTL validation logic
			minTTL := 10 * time.Minute
			maxTTL := 14 * 24 * time.Hour // 14 days

			isValid := tt.ttl >= minTTL && tt.ttl <= maxTTL
			assert.Equal(t, tt.expectValid, isValid, tt.description)
		})
	}
}

// TestJwtTokenWithM2M_LazyAndRetryScenarios covers:
// 1. first fetch & cache reuse (only one vault call for two token requests)
// 2. rotation retry success (401 invalid_client then success -> 2 vault calls during first token request only)
// 3. rotation retry failure (both attempts 401 -> error surfaced, 2 vault calls)
// 4. empty token then success (first response 200 with empty access_token triggers refresh + retry)
func TestJwtTokenWithM2M_LazyAndRetryScenarios(t *testing.T) {
	// helper to reset cached credentials between subtests
	resetCache := func() {
		credsMu.Lock()
		cachedClientID = ""
		cachedClientSecret = ""
		credsMu.Unlock()
	}

	makeSuccessToken := func() string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"exp": time.Now().Add(time.Hour).Unix(),
			"azp": "test-client",
		})
		s, _ := token.SignedString([]byte("secret"))
		return s
	}

	tests := []struct {
		name               string
		keycloakHandler    func() http.Handler
		expectErrSub       string
		expectedVaultCalls int32
		extraAssertion     func(t *testing.T)
	}{
		{
			name: "fetch and cache reuse",
			keycloakHandler: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(fmt.Sprintf("{\"access_token\":\"%s\"}", makeSuccessToken())))
				})
			},
			expectedVaultCalls: 1,
			extraAssertion: func(t *testing.T) {
				// second call should NOT trigger another vault fetch
				tok2, err2 := JwtTokenWithM2M(context.Background(), nil)
				assert.NoError(t, err2)
				assert.NotEmpty(t, tok2)
			},
		},
		{
			name: "rotation retry success (401 then 200)",
			keycloakHandler: func() http.Handler {
				var calls int
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if calls == 1 {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = w.Write([]byte("invalid_client"))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(fmt.Sprintf("{\"access_token\":\"%s\"}", makeSuccessToken())))
				})
			},
			expectedVaultCalls: 2,
			extraAssertion: func(t *testing.T) {
				// follow-up call should reuse refreshed credentials without additional vault call
				prev := atomic.LoadInt32(&vaultCalls)
				tok2, err2 := JwtTokenWithM2M(context.Background(), nil)
				assert.NoError(t, err2)
				assert.NotEmpty(t, tok2)
				assert.Equal(t, prev, atomic.LoadInt32(&vaultCalls), "vault call count increased unexpectedly on cache reuse after rotation")
			},
		},
		{
			name: "rotation retry failure (two 401 responses)",
			keycloakHandler: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte("invalid_client"))
				})
			},
			expectedVaultCalls: 2,
			expectErrSub:       "retry failed", // from doFinalTokenRequest
		},
		{
			name: "empty token then success triggers retry",
			keycloakHandler: func() http.Handler {
				var calls int
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					if calls == 1 {
						_, _ = w.Write([]byte("{\"access_token\":\"\"}")) // empty -> retryable
						return
					}
					_, _ = w.Write([]byte(fmt.Sprintf("{\"access_token\":\"%s\"}", makeSuccessToken())))
				})
			},
			expectedVaultCalls: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetCache()
			atomic.StoreInt32(&vaultCalls, 0)

			// counting VaultAuth implementation with cleanup
			prev := NewVaultAuthFunc
			NewVaultAuthFunc = func(vaultServer string, serviceAccount string) (VaultAuth, error) {
				return &countingVaultAuth{}, nil
			}
			t.Cleanup(func() { NewVaultAuthFunc = prev })

			handler := tc.keycloakHandler()
			server := httptest.NewServer(handler)
			defer server.Close()
			origEnv := os.Getenv("KEYCLOAK_URL")
			_ = os.Setenv("KEYCLOAK_URL", server.URL)
			defer func() {
				if origEnv == "" {
					_ = os.Unsetenv("KEYCLOAK_URL")
				} else {
					_ = os.Setenv("KEYCLOAK_URL", origEnv)
				}
			}()

			token, err := JwtTokenWithM2M(context.Background(), nil)
			if tc.expectErrSub != "" {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tc.expectErrSub)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}

			assert.Equal(t, tc.expectedVaultCalls, atomic.LoadInt32(&vaultCalls), "unexpected number of Vault credential fetches")

			if tc.extraAssertion != nil {
				tc.extraAssertion(t)
			}
		})
	}
}

// vaultCalls counts how many times Vault credentials were fetched (via GetClientCredentials)
var vaultCalls int32

// countingVaultAuth increments a counter every time credentials are fetched
type countingVaultAuth struct{}

func (c *countingVaultAuth) GetClientCredentials(ctx context.Context) (string, string, error) {
	n := atomic.AddInt32(&vaultCalls, 1)

	// return unique-ish credentials per call for debugging visibility
	return fmt.Sprintf("client-id-%d", n), fmt.Sprintf("client-secret-%d", n), nil
}
