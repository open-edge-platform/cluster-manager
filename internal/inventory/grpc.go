// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"errors"
	"log/slog"
	"time"

	computev1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/compute/v1"
	inventoryv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/inventory/v1"
	osv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/os/v1"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/client"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/validator"
)

const (
	clientName              = "ClusterManagerInventoryClient"
	defaultInventoryTimeout = 5 * time.Second
)

var (
	GetInventoryClientFunc = client.NewTenantAwareInventoryClient
)

// InventoryClient is a tenant-aware grpc client for the inventory service
type InventoryClient struct {
	client client.TenantAwareInventoryClient
	events chan *client.WatchEvents
	term   chan bool
}

// clientInstance is the singleton instance of the inventory client
var clientInstance *InventoryClient

// NewInventoryClientWithOptions gets or creates a singleton inventory client with the given options
func NewInventoryClientWithOptions(opt Options) (*InventoryClient, error) {
	if clientInstance != nil {
		slog.Info("inventory client already started")
		return clientInstance, nil
	}

	eventsWatcher := make(chan *client.WatchEvents)
	taic, err := GetInventoryClientFunc(context.Background(), client.InventoryClientConfig{
		Name:                      clientName,
		Address:                   opt.inventoryAddress,
		Events:                    eventsWatcher,
		AbortOnUnknownClientError: true,
		ClientKind:                inventoryv1.ClientKind_CLIENT_KIND_API,
		ResourceKinds:             []inventoryv1.ResourceKind{inventoryv1.ResourceKind_RESOURCE_KIND_HOST},
		EnableTracing:             opt.enableTracing,
		EnableMetrics:             opt.enableMetrics,
		Wg:                        opt.wg,
		SecurityCfg:               &client.SecurityConfig{Insecure: true},
	})
	if err != nil {
		slog.Warn("failed to start inventory client", "error", err)
		return nil, err
	}

	slog.Info("inventory client started")

	return &InventoryClient{client: taic, events: eventsWatcher, term: make(chan bool)}, nil
}

// GetHostTrustedCompute returns true if the host has secure boot and full disk encryption enabled
func (c *InventoryClient) GetHostTrustedCompute(ctx context.Context, tenantId, hostUuid string) (bool, error) {
	host, err := c.getHost(ctx, tenantId, hostUuid)
	if err != nil {
		return false, err
	}

	return host.Instance.SecurityFeature == osv1.SecurityFeature_SECURITY_FEATURE_SECURE_BOOT_AND_FULL_DISK_ENCRYPTION, nil
}

// getHost returns the host resource for the given tenant and host uuid
func (c *InventoryClient) getHost(ctx context.Context, tenantId, hostUuid string) (*computev1.HostResource, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultInventoryTimeout)
	defer cancel()

	slog.Debug("getting host", "tenantId", tenantId, "hostUuid", hostUuid)

	host, err := c.client.GetHostByUUID(ctx, tenantId, hostUuid)
	if err != nil {
		slog.Warn("failed to get host by uuid", "error", err, "tenantId", tenantId, "hostUuid", hostUuid)
		return nil, err
	}

	if err := c.validateHostResource(host); err != nil {
		slog.Warn("failed to validate host resource", "error", err, "tenantId", tenantId, "hostUuid", hostUuid)
		return nil, err
	}

	return host, nil
}

// validateHostResource validates the host resource and grpc message
func (c *InventoryClient) validateHostResource(host *computev1.HostResource) error {
	if host == nil {
		return errors.New("empty host resource")
	}

	if err := validator.ValidateMessage(host); err != nil {
		return err
	}

	return nil
}

// noopInventoryClient is a no-op implementation of the InventoryClient interface
type noopInventoryClient struct{}

// NewNoopInventoryClient returns a new no-op InventoryClient
func NewNoopInventoryClient() *noopInventoryClient {
	return &noopInventoryClient{}
}

// GetHostTrustedCompute is a no-op implementation of the InventoryClient's GetHostTrustedCompute method that always returns false
func (auth noopInventoryClient) GetHostTrustedCompute(ctx context.Context, tenantId, hostUuid string) (bool, error) {
	return false, nil
}

/*
// TODO: propegate host label updates to edge nodes
// WatchHosts watches for host resource events and calls the callback function
// with the host resource as an argument.
func (c *InventoryClient) WatchHosts(callback func(*computev1.HostResource)) {
	go func() {
		for {
			select {
			case event, ok := <-c.events:
				if !ok {
					slog.Warn("events channel closed")
					return
				}

				host := event.Event.Resource.GetHost()
				if err := c.validateHostResource(host); err != nil {
					slog.Warn("failed to validate host resource", "error", err)
					continue
				}

				if event.Event.EventKind != inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED {
					slog.Warn("unexpected event kind", "kind", event.Event.EventKind)
					continue
				}

				callback(host)
			case <-c.term:
				slog.Debug("inventory client stopping, exiting watch loop")
				return
			}
		}
	}()
}

// TODO: implement termination event handling in main.go
// IMPORTANT: always close the Inventory client in case of errors
// or signals like syscall.SIGTERM, syscall.SIGINT etc.
func (c *InventoryClient) close() {
	close(c.term)

	if err := c.Client.Close(); err != nil {
		slog.Error("failed to stop inventory client", "error", err)
	}

	slog.Info("inventory client stopped")
}
*/
