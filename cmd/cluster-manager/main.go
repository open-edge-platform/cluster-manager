// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/logger"
	"github.com/open-edge-platform/cluster-manager/v2/internal/mocks"
	"github.com/open-edge-platform/cluster-manager/v2/internal/multitenancy"
	"github.com/open-edge-platform/cluster-manager/v2/internal/rest"
	"github.com/open-edge-platform/cluster-manager/v2/internal/tenancyclient"
)

const controllerName = "cluster-manager"

const (
	tenancyRuntimeModeEnv = "TENANCY_RUNTIME_MODE"
	modeAuto              = "auto"
	modeLegacy            = "legacy"
	modePoller            = "poller"
)

// version injected at build time
var version string

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mock-server" {
		mocks.RunAuthServer()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		url := "http://localhost:8080/v2/healthz"
		if len(os.Args) > 2 {
			url = os.Args[2]
		}
		resp, err := http.Get(url) //nolint:gosec,noctx // health probe to fixed local endpoint
		if err != nil || resp.StatusCode/100 != 2 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	slog.Info("Cluster Manager started", "version", version)

	config := config.ParseConfig()
	if err := config.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("Cluster Manager configuration ", "config", config)

	logger.InitializeLogger(config)
	initializeSystemLabels(config)

	// Create a root context that is canceled on SIGTERM or SIGINT so background
	// goroutines (e.g. the tenancy poller) can shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initializeMultitenancy(ctx, config)

	k8sclient := initializeK8sClient()

	auth, err := rest.GetAuthenticator(config)
	if err != nil {
		slog.Error("failed to get authenticator", "error", err)
		os.Exit(4)
	}

	inv, err := rest.GetInventory(config, k8sclient)
	if err != nil {
		slog.Error("failed to start inventory client", "error", err)
		os.Exit(7)
	}

	s := rest.NewServer(k8sclient.Dyn, rest.WithAuth(auth), rest.WithConfig(config), rest.WithInventory(inv))
	if err := s.Serve(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(5)
	}
}

func initializeSystemLabels(config *config.Config) {
	if len(config.SystemLabelsPrefixes) > 0 {
		slog.Info(fmt.Sprintf("overriding system labels prefixes with %v", config.SystemLabelsPrefixes))
		labels.OverrideSystemPrefixes(config.SystemLabelsPrefixes)
	}
}

func initializeMultitenancy(ctx context.Context, config *config.Config) {
	multitenancy.SetDefaultTemplate(config.DefaultTemplate)

	if config.DisableMultitenancy {
		return
	}

	handler, err := multitenancy.NewDatamodelClient()
	if err != nil {
		slog.Error("failed to initialize tenancy datamodel client", "error", err)
		os.Exit(2)
	}

	runtimeMode := strings.ToLower(strings.TrimSpace(os.Getenv(tenancyRuntimeModeEnv)))
	if runtimeMode == "" {
		runtimeMode = modeAuto
	}

	if runtimeMode == modeLegacy || runtimeMode == modeAuto {
		if err := handler.Start(); err == nil {
			slog.Info("multitenancy running in legacy watcher mode")
			go func() {
				<-ctx.Done()
				handler.Stop()
			}()
			return
		} else if runtimeMode == modeLegacy {
			slog.Error("legacy watcher mode failed", "error", err)
			os.Exit(2)
		} else {
			slog.Warn("legacy watcher mode unavailable, falling back to poller mode", "error", err)
		}
	}

	if runtimeMode != modePoller && runtimeMode != modeAuto {
		slog.Error("invalid tenancy runtime mode", "mode", runtimeMode, "allowed", []string{modeAuto, modeLegacy, modePoller})
		os.Exit(2)
	}

	candidates := []string{}
	if env := os.Getenv("TENANT_MANAGER_URL"); env != "" {
		candidates = append(candidates, env)
	}
	if config.ProjectServiceURL != "" {
		candidates = append(candidates, config.ProjectServiceURL)
	}
	candidates = append(candidates,
		"http://svc-iam-nexus-api-gw.orch-iam.svc.cluster.local:8082",
		"http://tenancy-manager.orch-iam.svc:8080",
	)

	tenantManagerURL := tenancyclient.PickReachableURL(candidates)
	slog.Info("resolved tenancy manager url", "url", tenantManagerURL)

	tokenProvider := func(ctx context.Context) (string, error) {
		return auth.JwtTokenWithM2M(ctx, nil)
	}

	poller, err := tenancyclient.NewAuthPoller(tenantManagerURL, controllerName, handler,
		tokenProvider,
		func(cfg *tenancyclient.PollerConfig) {
			cfg.OnError = func(err error, msg string) {
				slog.Error("tenancy poller error", "msg", msg, "error", err)
			}
		},
	)
	if err != nil {
		slog.Error("failed to create tenancy poller", "error", err)
		os.Exit(2)
	}

	go func() {
		if err := poller.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("tenancy poller stopped unexpectedly", "error", err)
		}
	}()
	slog.Info("multitenancy running in poller mode", "mode", runtimeMode)
}

func initializeK8sClient() *k8s.Client {
	k8sclient := k8s.New().WithInClusterConfig()
	if k8sclient == nil {
		slog.Error("failed to initialize k8s clientset")
		os.Exit(3)
	}
	return k8sclient
}
