// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package tenancyclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	libtenancy "github.com/open-edge-platform/orch-library/go/pkg/tenancy"
)

type noopHandler struct{}

func (noopHandler) HandleEvent(_ context.Context, _ libtenancy.Event) error {
	return nil
}

func TestDoGetSetsActiveProjectIDFromEnvOverride(t *testing.T) {
	t.Setenv(tenancyManagerActiveProjectIDEnv, "11111111-1111-1111-1111-111111111111")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(activeProjectIDHeader); got != "11111111-1111-1111-1111-111111111111" {
			t.Fatalf("expected active project id from env, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatal("expected Authorization header to be set")
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return signedToken(t, []string{"22222222-2222-2222-2222-222222222222_member"}), nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.doGet(context.Background(), server.URL+"/v1/events?controller=cluster-manager"); err != nil {
		t.Fatalf("doGet() error = %v", err)
	}
}

func TestDoGetSetsActiveProjectIDFromSingleProjectJWT(t *testing.T) {
	t.Setenv(tenancyManagerActiveProjectIDEnv, "")
	const expectedProjectID = "33333333-3333-3333-3333-333333333333"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(activeProjectIDHeader); got != expectedProjectID {
			t.Fatalf("expected active project id from jwt, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return signedToken(t, []string{expectedProjectID + "_member", "other-role"}), nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.doGet(context.Background(), server.URL+"/v1/events?controller=cluster-manager"); err != nil {
		t.Fatalf("doGet() error = %v", err)
	}
}

func TestDoGetSkipsActiveProjectIDForMultiProjectJWT(t *testing.T) {
	t.Setenv(tenancyManagerActiveProjectIDEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(activeProjectIDHeader); got != "" {
			t.Fatalf("expected no active project id for multi-project jwt, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return signedToken(t, []string{
			"44444444-4444-4444-4444-444444444444_member",
			"55555555-5555-5555-5555-555555555555_member",
		}), nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.doGet(context.Background(), server.URL+"/v1/events?controller=cluster-manager"); err != nil {
		t.Fatalf("doGet() error = %v", err)
	}
}

func TestReplayUsesLegacyEventsPathByDefault(t *testing.T) {
	t.Setenv(tenancyManagerEventsProjectPathEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/events"; got != want {
			t.Fatalf("expected path %q, got %q", want, got)
		}
		if got := r.URL.Query().Get("replay"); got != "true" {
			t.Fatalf("expected replay=true, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.replay(context.Background()); err != nil {
		t.Fatalf("replay() error = %v", err)
	}
}

func TestReplayUsesProjectScopedEventsPathWhenConfigured(t *testing.T) {
	t.Setenv(tenancyManagerEventsProjectPathEnv, "my-project")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/projects/my-project/events"; got != want {
			t.Fatalf("expected path %q, got %q", want, got)
		}
		if got := r.URL.Query().Get("replay"); got != "true" {
			t.Fatalf("expected replay=true, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.replay(context.Background()); err != nil {
		t.Fatalf("replay() error = %v", err)
	}
}

func TestPollUsesProjectScopedEventsPathWhenConfigured(t *testing.T) {
	t.Setenv(tenancyManagerEventsProjectPathEnv, "my-project")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/projects/my-project/events"; got != want {
			t.Fatalf("expected path %q, got %q", want, got)
		}
		if got := r.URL.Query().Get("after"); got != "42" {
			t.Fatalf("expected after=42, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != strconv.Itoa(DefaultPollerConfig().PollLimit) {
			t.Fatalf("expected limit=%d, got %q", DefaultPollerConfig().PollLimit, got)
		}
		_ = json.NewEncoder(w).Encode(eventsResponse{})
	}))
	defer server.Close()

	poller, err := NewAuthPoller(server.URL, "cluster-manager", noopHandler{}, func(context.Context) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("NewAuthPoller() error = %v", err)
	}

	if _, err := poller.poll(context.Background(), 42); err != nil {
		t.Fatalf("poll() error = %v", err)
	}
}

func signedToken(t *testing.T, roles []string) string {
	t.Helper()

	roleInterfaces := make([]interface{}, 0, len(roles))
	for _, role := range roles {
		roleInterfaces = append(roleInterfaces, role)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"realm_access": map[string]any{
			"roles": roleInterfaces,
		},
	})

	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	return signed
}
