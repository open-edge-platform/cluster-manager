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
	modeAuto              = "auto"
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
// It supports legacy watcher mode, poller mode, and auto fallback.
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
		runtimeMode = modeAuto
	}

	if runtimeMode == modeLegacy || runtimeMode == modeAuto {
		if err := handler.Start(); err == nil {
			slog.Info("multitenancy running in legacy watcher mode")
			go func() {
				<-ctx.Done()
				handler.Stop()
			}()
			return nil
		} else if runtimeMode == modeLegacy {
			return fmt.Errorf("legacy watcher mode failed: %w", err)
		} else {
			slog.Warn("legacy watcher mode unavailable, falling back to poller mode", "error", err)
		}
	}

	if runtimeMode != modePoller && runtimeMode != modeAuto {
		return fmt.Errorf("invalid tenancy runtime mode %q, allowed: %s, %s, %s", runtimeMode, modeAuto, modeLegacy, modePoller)
	}

	candidates := []string{}
	if env := os.Getenv("TENANT_MANAGER_URL"); env != "" {
		candidates = append(candidates, env)
	}
	if cfg.ProjectServiceURL != "" {
		candidates = append(candidates, cfg.ProjectServiceURL)
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
