// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/logger"
	"github.com/open-edge-platform/cluster-manager/v2/internal/mocks"
	"github.com/open-edge-platform/cluster-manager/v2/internal/multitenancy"
	"github.com/open-edge-platform/cluster-manager/v2/internal/rest"
	"github.com/open-edge-platform/orch-library/go/pkg/tenancy"
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
	initializeMultitenancy(config)

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

func initializeMultitenancy(config *config.Config) {
	multitenancy.SetDefaultTemplate(config.DefaultTemplate)

	handler, err := multitenancy.NewTenancyHandler()
	if err != nil {
		slog.Error("failed to initialize tenancy handler", "error", err)
		os.Exit(2)
	}

	tenantManagerURL := os.Getenv("TENANT_MANAGER_URL")
	if tenantManagerURL == "" {
		tenantManagerURL = "http://tenancy-manager.orch-iam:8080"
	}

	poller := tenancy.NewPoller(tenantManagerURL, "cluster-manager", handler,
		func(cfg *tenancy.PollerConfig) {
			cfg.OnError = func(err error, msg string) {
				slog.Error(msg, "error", err)
			}
		},
	)

	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	go func() {
		if err := poller.Run(pollerCtx); err != nil && pollerCtx.Err() == nil {
			slog.Error("tenancy poller stopped with error", "error", err)
		}
	}()

	// Cancel the poller on process exit.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		pollerCancel()
	}()
}

func initializeK8sClient() *k8s.Client {
	k8sclient := k8s.New().WithInClusterConfig()
	if k8sclient == nil {
		slog.Error("failed to initialize k8s clientset")
		os.Exit(3)
	}
	return k8sclient
}
