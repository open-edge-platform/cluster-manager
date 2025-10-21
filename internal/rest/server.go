// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/getkin/kin-openapi/openapi3filter"
	oapi_middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/inventory"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/metrics"
	cm_middleware "github.com/open-edge-platform/cluster-manager/v2/internal/middleware"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

var (
	// Ensure Server implements api.StrictServerInterface
	_ api.StrictServerInterface = (*Server)(nil)
)

// Authenticator is an interface that can be used to authenticate requests
type Authenticator interface {
	Authenticate(ctx context.Context, input *openapi3filter.AuthenticationInput) error
}

// Provider is an interface that can be used with an Authenticator to verify tokens
type Provider interface {
	GetSigningKey(kid string) (interface{}, error)
}

type Inventory interface {
	GetHostTrustedCompute(ctx context.Context, tenantId, hostUuid string) (bool, error)
	IsImmutable(ctx context.Context, tenantId, hostUuid string) (bool, error)
}

type Server struct {
	config    *config.Config
	auth      Authenticator
	k8sclient k8s.Client
	inventory Inventory
}

// NewServer creates a new Server instance
func NewServer(k8sclient k8s.Client, options ...func(*Server)) *Server {
	svr := &Server{
		config:    &config.Config{},
		auth:      auth.NewNoopAuthenticator(),
		k8sclient: k8sclient,
		inventory: inventory.NewNoopInventoryClient(),
	}

	for _, o := range options {
		o(svr)
	}

	return svr
}

// WithAuth is a functional option for configuring a Server with an Authenticator
func WithAuth(auth Authenticator) func(*Server) {
	return func(s *Server) {
		s.auth = auth
	}
}

// WithConfig is a functional option for configuring a Server with a Config
func WithConfig(cfg *config.Config) func(*Server) {
	return func(s *Server) {
		s.config = cfg
	}
}

// WithInventory is a functional option for configuring a Server with an InventoryClient
func WithInventory(inv Inventory) func(*Server) {
	return func(s *Server) {
		s.inventory = inv
	}
}

// Serve starts the server
func (s *Server) Serve() error {
	handler, err := s.ConfigureHandler()
	if err != nil {
		slog.Error("failed to initialize handler middleware", "error", err)
		return err
	}

	addr := "0.0.0.0:8080"
	slog.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		return err
	}

	return nil
}

// ConfigureHandler configures the server with necessary middleware and handlers
func (s *Server) ConfigureHandler() (http.Handler, error) {
	// handler already implements request validation via oapi request validator
	handler, err := s.getServerHandler()
	if err != nil {
		slog.Error("failed to get oapi handler", "error", err)
		return nil, err
	}

	return cm_middleware.Append(
		func(handler http.Handler) http.Handler {
			return cm_middleware.RequestDurationMetrics(metrics.ResponseTime, handler)
		},
		func(handler http.Handler) http.Handler {
			return cm_middleware.ResponseCounterMetrics(metrics.HttpResponseCounter, handler)
		},
		cm_middleware.Logger,
		cm_middleware.ProjectIDValidator)(handler), nil
}

// getServerHandler returns the base http handler with strict validation against the OpenAPI spec
func (s *Server) getServerHandler() (http.Handler, error) {
	// create the router for the metrics endpoint
	router := http.NewServeMux()

	if !s.config.DisableMetrics {
		router.Handle("/metrics", promhttp.HandlerFor(metrics.GetRegistry(), promhttp.HandlerOpts{}))
	}

	// create the openapi handler with existing router
	handler := api.HandlerWithOptions(api.NewStrictHandler(s, nil), api.StdHTTPServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error(err.Error(), "path", r.URL.Path, "method", r.Method)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)

			if err := json.NewEncoder(w).Encode(api.N400BadRequest{Message: ptr(err.Error())}); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
		},
	})

	// load the swagger spec
	swagger, err := api.GetSwagger()
	if err != nil {
		slog.Error("failed to get swagger spec", "error", err)
		return nil, err
	}
	swagger.Servers = nil

	// set up the request validator with authentication and error handling
	validator := oapi_middleware.OapiRequestValidatorWithOptions(swagger, &oapi_middleware.Options{
		Options: openapi3filter.Options{AuthenticationFunc: s.auth.Authenticate},
		ErrorHandler: func(w http.ResponseWriter, message string, code int) {
			slog.Error(message, "status", code)
			if http.StatusBadRequest == code {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)

				response := api.N400BadRequest{Message: &message}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					http.Error(w, "Failed to encode response", http.StatusInternalServerError)
				}
			} else {
				http.Error(w, message, code)
			}
		},
	})

	return validator(handler), nil
}

func GetAuthenticator(cfg *config.Config) (Authenticator, error) {
	if cfg.DisableAuth {
		slog.Warn("authentication/authorization is disabled")
		return auth.NewNoopAuthenticator(), nil
	}

	provider, err := auth.NewOidcProvider(cfg.OidcUrl)
	if err != nil {
		slog.Error("failed to initialize oidc authenticator", "error", err)
		return nil, err
	}

	if !cfg.OpaEnabled {
		slog.Warn("opa is not enabled")
		return auth.NewOidcAuthenticator(provider, nil)
	}

	opa, err := auth.NewOpaClient(cfg.OpaPort)
	if err != nil {
		return nil, fmt.Errorf("failed to create opa client: %w", err)
	}

	return auth.NewOidcAuthenticator(provider, opa)
}

func GetInventory(cfg *config.Config, k8sClient k8s.K8sWrapperClient) (Inventory, error) {
	if cfg.DisableInventory {
		slog.Warn("inventory integration is disabled")
		return inventory.NewNoopInventoryClient(), nil
	}

	return inventory.NewInventoryClientWithOptions(inventory.NewOptionsBuilder().
		WithWaitGroup(&sync.WaitGroup{}).
		WithInventoryAddress(cfg.InventoryAddress).
		WithTracing(false).
		WithMetrics(false).
		WithK8sClient(k8sClient).
		Build())
}
