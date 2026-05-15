// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/open-edge-platform/orch-library/go/pkg/middleware/projectcontext"
)

func TestProjectContextInjectionBeforeValidation(t *testing.T) {
	projectID := "12345678-1234-1234-1234-123456789012"
	req := httptest.NewRequest(http.MethodGet, "/v2/clusters", nil)
	req.Header.Set("Authorization", bearerTokenWithProjectRole(projectID))

	rr := httptest.NewRecorder()
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		if got := r.Header.Get("Activeprojectid"); got != projectID {
			t.Fatalf("expected injected project id %q, got %q", projectID, got)
		}
		_, _ = w.Write([]byte("handler called"))
	})

	chain := projectcontext.InjectActiveProjectID("", false)(ProjectIDValidator(handler))
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !handlerCalled {
		t.Fatal("expected downstream handler to be called")
	}
}

func bearerTokenWithProjectRole(projectID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"realm_access": map[string]any{
			"roles": []string{projectID + "_member"},
		},
	})

	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		panic(err)
	}

	return "Bearer " + signed
}
