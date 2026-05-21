// SPDX-FileCopyrightText: (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"errors"
	"testing"

	libtenancy "github.com/open-edge-platform/orch-library/go/pkg/tenancy"

	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/tenancyclient"
)

type fakeRuntimeHandler struct {
	startErr    error
	startCalls  int
	stopCalls   int
	handleCalls int
}

func (f *fakeRuntimeHandler) Start() error {
	f.startCalls++
	return f.startErr
}

func (f *fakeRuntimeHandler) Stop() {
	f.stopCalls++
}

func (f *fakeRuntimeHandler) HandleEvent(context.Context, libtenancy.Event) error {
	f.handleCalls++
	return nil
}

type fakeRuntimePoller struct {
	runCalls int
}

func (f *fakeRuntimePoller) Run(context.Context) error {
	f.runCalls++
	return nil
}

func TestInitializeRuntimeLegacyMode(t *testing.T) {
	t.Setenv(TenancyRuntimeModeEnv, modeLegacy)

	handler := &fakeRuntimeHandler{}

	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})

	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(string, string, libtenancy.Handler, func(context.Context) (string, error), ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		t.Fatal("poller constructor should not be called in legacy mode")
		return nil, nil
	}

	cfg := &config.Config{DisableMultitenancy: false}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err != nil {
		t.Fatalf("InitializeRuntime() error = %v", err)
	}

	if handler.startCalls != 1 {
		t.Fatalf("handler.Start() calls = %d, want 1", handler.startCalls)
	}
}

func TestInitializeRuntimeDefaultModeIsAutoAndUsesLegacyWhenAvailable(t *testing.T) {
	// Unset env to exercise default mode selection.
	t.Setenv(TenancyRuntimeModeEnv, "")

	handler := &fakeRuntimeHandler{}

	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})

	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(string, string, libtenancy.Handler, func(context.Context) (string, error), ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		t.Fatal("poller constructor should not be called when default auto mode can use legacy")
		return nil, nil
	}

	cfg := &config.Config{DisableMultitenancy: false}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err != nil {
		t.Fatalf("InitializeRuntime() error = %v", err)
	}

	if handler.startCalls != 1 {
		t.Fatalf("handler.Start() calls = %d, want 1", handler.startCalls)
	}
}

func TestInitializeRuntimeAutoUsesLegacyWhenAvailable(t *testing.T) {
	t.Setenv(TenancyRuntimeModeEnv, modeAuto)

	handler := &fakeRuntimeHandler{}

	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})

	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(string, string, libtenancy.Handler, func(context.Context) (string, error), ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		t.Fatal("poller constructor should not be called when legacy starts in auto mode")
		return nil, nil
	}

	cfg := &config.Config{DisableMultitenancy: false}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err != nil {
		t.Fatalf("InitializeRuntime() error = %v", err)
	}

	if handler.startCalls != 1 {
		t.Fatalf("handler.Start() calls = %d, want 1", handler.startCalls)
	}
}

func TestInitializeRuntimeAutoFallbackToPoller(t *testing.T) {
	t.Setenv(TenancyRuntimeModeEnv, modeAuto)
	t.Setenv("TENANT_MANAGER_URL", "http://127.0.0.1:1")

	handler := &fakeRuntimeHandler{startErr: errors.New("legacy unavailable")}
	pollerConstructed := false

	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})

	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(string, string, libtenancy.Handler, func(context.Context) (string, error), ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		pollerConstructed = true
		return &fakeRuntimePoller{}, nil
	}

	cfg := &config.Config{DisableMultitenancy: false}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err != nil {
		t.Fatalf("InitializeRuntime() error = %v", err)
	}

	if handler.startCalls != 1 {
		t.Fatalf("handler.Start() calls = %d, want 1", handler.startCalls)
	}
	if !pollerConstructed {
		t.Fatal("expected poller constructor to be called in auto fallback mode")
	}
}

func TestInitializeRuntimeInvalidMode(t *testing.T) {
	t.Setenv(TenancyRuntimeModeEnv, "somenonsense")

	handler := &fakeRuntimeHandler{}
	pollerConstructed := false
	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})
	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(string, string, libtenancy.Handler, func(context.Context) (string, error), ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		pollerConstructed = true
		return &fakeRuntimePoller{}, nil
	}

	cfg := &config.Config{DisableMultitenancy: false}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err == nil {
		t.Fatal("InitializeRuntime() expected error for invalid mode, got nil")
	}
	if handler.startCalls != 0 {
		t.Fatalf("handler.Start() calls = %d, want 0", handler.startCalls)
	}
	if pollerConstructed {
		t.Fatal("poller constructor should not be called for invalid mode")
	}
}

func TestInitializeRuntimePollerModeUsesConfiguredURL(t *testing.T) {
	t.Setenv(TenancyRuntimeModeEnv, modePoller)

	handler := &fakeRuntimeHandler{}
	wantURL := "http://configured-tenancy-manager:8080"
	gotURL := ""

	origNewHandler := newRuntimeHandler
	origNewPoller := newRuntimePoller
	t.Cleanup(func() {
		newRuntimeHandler = origNewHandler
		newRuntimePoller = origNewPoller
	})

	newRuntimeHandler = func() (runtimeHandler, error) {
		return handler, nil
	}
	newRuntimePoller = func(tenantManagerURL, _ string, _ libtenancy.Handler, _ func(context.Context) (string, error), _ ...func(*tenancyclient.PollerConfig)) (runtimePoller, error) {
		gotURL = tenantManagerURL
		return &fakeRuntimePoller{}, nil
	}

	cfg := &config.Config{DisableMultitenancy: false, ProjectServiceURL: wantURL}
	if err := InitializeRuntime(context.Background(), cfg, "cluster-manager"); err != nil {
		t.Fatalf("InitializeRuntime() error = %v", err)
	}

	if gotURL != wantURL {
		t.Fatalf("poller URL = %q, want %q", gotURL, wantURL)
	}
	if handler.startCalls != 0 {
		t.Fatalf("handler.Start() calls = %d, want 0", handler.startCalls)
	}
}
