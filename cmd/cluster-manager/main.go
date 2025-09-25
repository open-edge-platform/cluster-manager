// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	initializeSystemLabels(config)
	initializeMultitenancy(config)

	k8sclient := initializeK8sClient()

	authenticator, err := rest.GetAuthenticator(config)
	if err != nil {
		slog.Error("failed to get authenticator", "error", err)
		os.Exit(4)
	}

	inv, err := rest.GetInventory(config, k8sclient)
	if err != nil {
		slog.Error("failed to start inventory client", "error", err)
		os.Exit(7)
	}

	// retrieve client credentials from Vault
	clientId, vaultOK := initVaultClientCredentials(config)
	if !vaultOK {
		slog.Warn("Vault unavailable: disabling kubeconfig token renewal and custom TTL enforcement")
		config.DisableCustomTTL = true
	}
	// handle TTL enforcement
	handleTTLEnforcement(config, clientId, vaultOK)

	s := rest.NewServer(k8sclient.Dyn, rest.WithAuth(authenticator), rest.WithConfig(config), rest.WithInventory(inv))
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
}

func initializeK8sClient() *k8s.Client {
	k8sclient := k8s.New().WithInClusterConfig()
	if k8sclient == nil {
		slog.Error("failed to initialize k8s clientset")
		os.Exit(3)
	}
	return k8sclient
}

func handleTTLEnforcement(config *config.Config, clientId string, vaultOK bool) {
	if config.DisableAuth || !vaultOK {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := auth.JwtTokenWithM2M(ctx, &config.DefaultKubeconfigTTL)
	if err != nil {
		slog.Warn("failed to obtain admin token; skipping TTL enforcement", "error", err)
		return
	}

	if config.DisableCustomTTL {
		// override the existed keycloak access token lifespan
		auth.ClearClientAccessTokenTTL(context.Background(), config.OidcUrl, "", clientId, token, slog.Default())
	} else {
		// clear any existing override so client inherits realm default
		auth.EnforceClientAccessTokenTTL(context.Background(), config.OidcUrl, "", clientId, config.DefaultKubeconfigTTL, token, slog.Default())
	}
}

// initVaultClientCredentials fetches Keycloak client credentials from Vault if auth is enabled
// on failure (non-fatal) token renewal and custom TTL enforcement are disabled
func initVaultClientCredentials(cfg *config.Config) (string, bool) {
	if cfg.DisableAuth {
		return "", false
	}

	vaultAuth, err := auth.NewVaultAuth(auth.VaultServer, auth.ServiceAccount)
	if err != nil {
		slog.Warn("vault init failed", "error", err)
		return "", false
	}

	clientId, _, err := vaultAuth.GetClientCredentials(context.Background())
	if err != nil {
		slog.Warn("failed to fetch client credentials from Vault", "error", err)
		return "", false
	}
	return clientId, true
}
