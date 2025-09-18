// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/logger"
	"github.com/open-edge-platform/cluster-manager/v2/internal/multitenancy"
	"github.com/open-edge-platform/cluster-manager/v2/internal/rest"
)

// version injected at build time
var version string

func main() {
	slog.Info("Cluster Manager started", "version", version)

	config := config.ParseConfig()
	if err := config.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("Cluster Manager configuration ", "config", config)

	logger.InitializeLogger(config)

	if len(config.SystemLabelsPrefixes) > 0 {
		slog.Info(fmt.Sprintf("overriding system labels prefixes with %v", config.SystemLabelsPrefixes))
		labels.OverrideSystemPrefixes(config.SystemLabelsPrefixes)
	}

	multitenancy.SetDefaultTemplate(config.DefaultTemplate)

	if !config.DisableMultitenancy {
		// TODO? may need to be initialized after server as all resource handling is done in the server
		tdm, err := multitenancy.NewDatamodelClient()
		if err != nil {
			slog.Error("failed to initialize tenancy datamodel client", "error", err)
			os.Exit(2)
		}
		if err = tdm.Start(); err != nil {
			slog.Error("failed to start tenancy datamodel client", "error", err)
			os.Exit(2)
		}
	}

	k8sclient := k8s.New().WithInClusterConfig()
	if k8sclient == nil {
		slog.Error("failed to initialize k8s clientset")
		os.Exit(3)
	}

	// Initialize VaultAuth and fetch client credentials only when authentication is enabled
	if !config.DisableAuth {
		vaultAuth, err := auth.NewVaultAuth(auth.VaultServer, auth.ServiceAccount)
		if err != nil {
			slog.Error("failed to initialize VaultAuth", "error", err)
			os.Exit(4)
		}

		_, _, err = vaultAuth.GetClientCredentials(context.Background())
		if err != nil {
			slog.Error("failed to fetch client credentials from Vault", "error", err)
			os.Exit(4)
		}
		// Credentials retrieved successfully - do not log sensitive values
		slog.Info("Successfully retrieved client credentials from Vault")
	} else {
		slog.Info("Authentication disabled, skipping Vault initialization")
	}

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
