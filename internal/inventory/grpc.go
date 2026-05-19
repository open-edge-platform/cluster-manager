// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	computev1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/compute/v1"
	inventoryv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/inventory/v1"
	osv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/os/v1"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/client"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/validator"
)

const (
	clientName              = "ClusterManagerInventoryClient"
	defaultInventoryTimeout = 5 * time.Second
	autoCreatedLabelValue   = "true"
)

var (
	GetInventoryClientFunc = client.NewTenantAwareInventoryClient
)

// InventoryClient is a tenant-aware grpc client for the inventory service
type InventoryClient struct {
	client    client.TenantAwareInventoryClient
	events    chan *client.WatchEvents
	term      chan bool
	k8sclient k8s.K8sWrapperClient
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
	cli.k8sclient = opt.k8sClient
	cli.WatchHosts(events.NewSink(context.TODO()))
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

// EnableAirGapInstall returns true if the host OS type is immutable (e.g. EMT with pre-installed K8s packages)
func (c *InventoryClient) IsImmutable(ctx context.Context, tenantId, hostUuid string) (bool, error) {
	host, err := c.getHost(ctx, tenantId, hostUuid)
	if err != nil {
		return false, err
	}

	if host.Instance == nil {
		return false, errors.New("host instance is nil")
	}

	if host.Instance.Os == nil {
		return false, errors.New("host instance os is nil")
	}

	// The expectation is when the host OS is immutable, we expect the k3s packages to be bundled as part of the
	// OS image. So, we assume that the cluster is installed in air-gap mode.
	// return host.Instance.Os.OsType == osv1.OsType_OS_TYPE_IMMUTABLE, nil
	// Always return false for now as we don't support immutable EMT with pre-installed K3s packages yet
	return false, nil
}

// getHost returns the host resource for the given tenant and host uuid
func (c *InventoryClient) getHost(ctx context.Context, tenantId, hostUuid string) (*computev1.HostResource, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultInventoryTimeout)
	defer cancel()

	slog.Debug("getting host", "tenantId", tenantId, "hostUuid", hostUuid)

	host, err := c.client.GetHostByUUID(ctx, tenantId, hostUuid)
	if err != nil {
		slog.Warn("failed to get host by uuid, attempting with resource id", "error", err, "tenantId", tenantId, "hostId", hostUuid)

		response, err := c.client.Get(ctx, tenantId, hostUuid)
		if err != nil {
			slog.Warn("failed to get host by resourceId", "error", err, "tenantId", tenantId, "hostId", hostUuid)
			return nil, err
		}

		resource := response.GetResource()
		if resource == nil {
			slog.Warn("response resource is nil", "tenantId", tenantId, "hostUuid", hostUuid)
			return nil, errors.New("response resource is nil")
		}
		host = resource.GetHost()
		if host == nil {
			slog.Warn("host in response resource is nil", "tenantId", tenantId, "hostUuid", hostUuid)
			return nil, errors.New("host in response resource is nil")
		}
		slog.Debug("success in getting resourceId", "tenantId", tenantId, "hostUuid", hostUuid)
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
type noopInventoryClient struct {
}

// NewNoopInventoryClient returns a new no-op InventoryClient
func NewNoopInventoryClient() *noopInventoryClient {
	return &noopInventoryClient{}
}

// GetHostTrustedCompute is a no-op implementation of the InventoryClient's GetHostTrustedCompute method that always returns false
func (auth noopInventoryClient) GetHostTrustedCompute(ctx context.Context, tenantId, hostUuid string) (bool, error) {
	return false, nil
}

// IsImmutable is a no-op implementation of the InventoryClient's IsImmutable method that always returns false
func (auth noopInventoryClient) IsImmutable(ctx context.Context, tenantId, hostUuid string) (bool, error) {
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
				// Delete the assigned cluster only when it is explicitly marked as auto-created.
				if host.CurrentState == computev1.HostState_HOST_STATE_UNTRUSTED {
					slog.Info("host is deauthenticating, performing cleanup", "hostId", host.ResourceId, "tenantId", host.TenantId)
					c.cleanupHostAssignedCluster(context.TODO(), host, "host-state-untrusted")
					continue
				}
				switch event.Event.EventKind {
				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_CREATED:
					slog.Debug("host created event", "name", host.Name, "hostid", host.ResourceId)
					hostEvents <- HostCreated{
						HostEventBase: HostEventBase{
							HostId:    host.ResourceId,
							ProjectId: host.TenantId,
						},
					}
				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED:
					slog.Debug("host deleted event", "name", host.Name, "hostid", host.ResourceId)
					c.cleanupHostAssignedCluster(context.TODO(), host, "host-event-deleted")
					hostEvents <- HostDeleted{
						HostEventBase: HostEventBase{
							HostId:    host.ResourceId,
							ProjectId: host.TenantId,
						},
					}

				case inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED:
					slog.Debug("host updated event", "name", host.Name, "hostid", host.ResourceId)

					l, err := JsonStringToMap(host.Metadata)
					if err != nil {
						slog.Warn("failed to convert json string to map", "error", err, "metadata", host.Metadata)
						continue
					}

					hostEvents <- &HostUpdated{
						HostEventBase: HostEventBase{
							HostId:    host.ResourceId,
							ProjectId: host.TenantId,
						},
						Labels: l,
						K8scli: c.k8sclient,
					}
				}

			case <-c.term:
				slog.Debug("inventory client stopping, exiting watch loop")
				return
			}
		}
	}()
}

func (c *InventoryClient) cleanupHostAssignedCluster(ctx context.Context, host *computev1.HostResource, reason string) {
	machine, err := c.k8sclient.GetMachineByHostID(ctx, host.TenantId, host.ResourceId)
	if err != nil {
		if !isNotFoundErr(err) {
			slog.Warn("failed to get machine by host id", "error", err, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
			return
		}

		slog.Info("machine lookup by node ref missed, trying provider annotation fallback", "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
		machine, err = c.k8sclient.GetMachineByProviderHostID(ctx, host.TenantId, host.ResourceId)
		if err != nil {
			if isNotFoundErr(err) {
				slog.Info("no machine found for host after fallback lookup, skipping cleanup", "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
				return
			}
			slog.Warn("failed fallback machine lookup by provider host id", "error", err, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
			return
		}

		slog.Info("resolved machine via provider annotation fallback", "hostId", host.ResourceId, "tenantId", host.TenantId, "cluster", machine.Spec.ClusterName, "reason", reason)
	}

	clusterName := machine.Spec.ClusterName
	if clusterName == "" {
		slog.Debug("machine has no assigned cluster, skipping cleanup", "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
		return
	}

	cluster, err := c.k8sclient.GetCluster(ctx, host.TenantId, clusterName)
	if err != nil {
		if errors.Is(err, k8s.ErrClusterNotFound) || isNotFoundErr(err) {
			slog.Info("assigned cluster already absent, skipping cleanup", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
			return
		}
		slog.Warn("failed to fetch assigned cluster", "error", err, "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
		return
	}

	if cluster == nil {
		slog.Warn("assigned cluster lookup returned nil", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
		return
	}

	if !isAutoCreatedCluster(cluster.Labels) {
		slog.Info("assigned cluster is not auto-created, skipping cleanup", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
		return
	}

	// TODO: for multi-node, if cluster replicas > 1, remove IntelMachineBinding and decrement replicas
	slog.Info("deleting assigned auto-created cluster", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
	if err := c.k8sclient.DeleteCluster(ctx, host.TenantId, clusterName); err != nil {
		if errors.Is(err, k8s.ErrClusterNotFound) || isNotFoundErr(err) {
			slog.Info("assigned cluster already deleted", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
			return
		}
		slog.Warn("failed to delete assigned cluster", "error", err, "cluster", clusterName, "tenantId", host.TenantId, "reason", reason)
		return
	}

	slog.Info("deleted assigned auto-created cluster", "cluster", clusterName, "hostId", host.ResourceId, "tenantId", host.TenantId, "reason", reason)
}

func isAutoCreatedCluster(clusterLabels map[string]string) bool {
	if clusterLabels == nil {
		return false
	}

	v, exists := clusterLabels[labels.AutoCreatedLabelKey]
	if !exists {
		return false
	}

	isAutoCreated, err := strconv.ParseBool(v)
	if err != nil {
		return strings.EqualFold(v, autoCreatedLabelValue)
	}

	return isAutoCreated
}

func isNotFoundErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func JsonStringToMap(jsonString string) (map[string]string, error) {
	out := make(map[string]string)
	if jsonString == "" {
		return out, nil
	}
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
