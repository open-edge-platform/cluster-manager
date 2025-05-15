// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
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
	client    client.TenantAwareInventoryClient
	events    chan *client.WatchEvents
	term      chan bool
	k8sclient *k8s.Client
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

	cli, err := &InventoryClient{client: taic, events: eventsWatcher, term: make(chan bool)}, nil
	cli.WatchHosts(events.Sink())
	return cli, err
}

// GetHostTrustedCompute returns true if the host has secure boot and full disk encryption enabled
func (c *InventoryClient) GetHostTrustedCompute(ctx context.Context, tenantId, hostUuid string) (bool, error) {
	host, err := c.getHost(ctx, tenantId, hostUuid)
	if err != nil {
		return false, err
	}

	if host.Instance == nil {
		return false, errors.New("host instance is nil")
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

// WatchHosts watches for host resource events and sends them to the given channel
func (c *InventoryClient) WatchHosts(hostEvents chan<- events.Event) {
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

				switch event.Event.EventKind {
				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_CREATED:
					slog.Debug("host created event", "name", host.Name, "hostid", host.ResourceId)
					hostEvents <- events.HostCreated{
						HostId:    host.ResourceId,
						ProjectId: host.TenantId,
					}
				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED:
					slog.Debug("host deleted event", "name", host.Name, "hostid", host.ResourceId)
					hostEvents <- events.HostDeletedEvent{
						HostId:    host.ResourceId,
						ProjectId: host.TenantId,
					}

				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED:
					slog.Debug("host updated event", "name", host.Name, "hostid", host.ResourceId)

					l, err := JsonStringToMap(host.Metadata)
					if err != nil {
						slog.Warn("failed to convert json string to map", "error", err, "metadata", host.Metadata)
						continue
					}

					hostEvents <- &events.HostUpdate{
						HostId:    host.ResourceId,
						ProjectId: host.ResourceId,
						Labels:    l,
						K8scli:    c.k8sclient,
					}
				}

			case <-c.term:
				slog.Debug("inventory client stopping, exiting watch loop")
				return
			}
		}
	}()
}

func JsonStringToMap(jsonString string) (map[string]string, error) {
	out := make(map[string]string)
	// Unmarshal the JSON string into a slice of structs
	var result []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, err
	}
	// Iterate over the result and populate the map
	for _, item := range result {
		out[item.Key] = item.Value
	}

	return out, nil
}

/*
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
