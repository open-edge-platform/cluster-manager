// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	libtenancy "github.com/open-edge-platform/orch-library/go/pkg/tenancy"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/tenancyclient"
)

const (
	TenancyRuntimeModeEnv = "TENANCY_RUNTIME_MODE"
	modeLegacy            = "legacy"
	modePoller            = "poller"
)

type runtimeHandler interface {
	libtenancy.Handler
	Start() error
	Stop()
}

type runtimePoller interface {
	Run(ctx context.Context) error
}

var (
	newRuntimeHandler = func() (runtimeHandler, error) {
		return NewDatamodelClient()
	}
	newRuntimePoller = func(tenantManagerURL, controllerName string, handler libtenancy.Handler, tokenProvider func(ctx context.Context) (string, error), opts ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		return tenancyclient.NewAuthPoller(tenantManagerURL, controllerName, handler, tokenProvider, opts...)
	}
)

// InitializeRuntime configures and starts multitenancy runtime behavior.
// It supports explicit legacy watcher mode and explicit poller mode.
func InitializeRuntime(ctx context.Context, cfg *config.Config, controllerName string) error {
	SetDefaultTemplate(cfg.DefaultTemplate)

	if cfg.DisableMultitenancy {
		return nil
	}

	handler, err := newRuntimeHandler()
	if err != nil {
		return fmt.Errorf("failed to initialize tenancy datamodel client: %w", err)
	}

	runtimeMode := strings.ToLower(strings.TrimSpace(os.Getenv(TenancyRuntimeModeEnv)))
	if runtimeMode == "" {
		runtimeMode = modeLegacy
	}

	if runtimeMode != modeLegacy && runtimeMode != modePoller {
		return fmt.Errorf("invalid tenancy runtime mode %q, allowed: %s, %s", runtimeMode, modeLegacy, modePoller)
	}

	if runtimeMode == modeLegacy {
		if err := handler.Start(); err != nil {
			return fmt.Errorf("legacy watcher mode failed: %w", err)
		}
		slog.Info("multitenancy running in legacy watcher mode")
		go func() {
			<-ctx.Done()
			handler.Stop()
		}()
		return nil
	}

	tenantManagerURL := strings.TrimSpace(os.Getenv("TENANT_MANAGER_URL"))
	if tenantManagerURL == "" {
		tenantManagerURL = strings.TrimSpace(cfg.ProjectServiceURL)
	}
	if tenantManagerURL == "" {
		return fmt.Errorf("tenant manager url must be configured for %s mode via TENANT_MANAGER_URL or --nexus-api-url", runtimeMode)
	}
	slog.Info("using configured tenancy manager url", "url", tenantManagerURL)

	tokenProvider := func(ctx context.Context) (string, error) {
		return auth.JwtTokenWithM2M(ctx, nil)
	}

	poller, err := newRuntimePoller(tenantManagerURL, controllerName, handler,
		tokenProvider,
		func(cfg *tenancyclient.PollerConfig) {
			cfg.OnError = func(err error, msg string) {
				slog.Error("tenancy poller error", "msg", msg, "error", err)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create tenancy poller: %w", err)
	}

	go func() {
		if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("tenancy poller stopped unexpectedly", "error", err)
		}
	}()
	slog.Info("multitenancy running in poller mode", "mode", runtimeMode)

	return nil
}
