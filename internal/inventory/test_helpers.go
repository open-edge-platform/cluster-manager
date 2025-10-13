// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/client"
)

// NewTestInventoryClient creates an inventory client for testing with injectable dependencies
func NewTestInventoryClient(k8sClient k8s.K8sWrapperClient, tenantClient client.TenantAwareInventoryClient) *InventoryClient {
	return &InventoryClient{
		client:    tenantClient,
		events:    make(chan *client.WatchEvents, 1),
		term:      make(chan bool),
		k8sclient: k8sClient,
	}
}

// InjectEvent injects a test event into the inventory client's events channel
func (c *InventoryClient) InjectEvent(event *client.WatchEvents) {
	c.events <- event
}

// CloseEvents closes the events channel to stop watching
func (c *InventoryClient) CloseEvents() {
	close(c.events)
}
