// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConcurrentEventHandling(t *testing.T) {
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Create output channels to track results
	outputs := make([]chan error, 10)
	for i := range outputs {
		outputs[i] = make(chan error, 1)
	}

	// Machines to be updated
	machines := make([]*unstructured.Unstructured, 10)

	// Launch many events concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Test data
			si := strconv.Itoa(i)
			projectID := "64e797f6-db23-445e-b606-4228d4f1c2bd" + si
			hostID := "host-12345" + si
			labels := map[string]string{"key" + si: si}

			// Setup mock k8s client and resources
			var cli *k8s.Client
			cli, machines[i] = setupMockK8sClient(t, projectID, hostID)

			event := createHostUpdatedEvent(hostID, projectID, labels, outputs[i], cli)
			sink <- event
		}(i)
	}

	// Wait for all events to be sent
	wg.Wait()

	// Verify all events were processed
	for i, out := range outputs {
		select {
		case err := <-out:
			require.NoError(t, err, "Event %d failed", i)
		case <-time.After(5 * time.Second):
			t.Errorf("Timeout waiting for event %d", i)
		}
	}

	// Verify the results
	for i, machine := range machines {
		require.Equal(t, 1, len(machine.GetLabels()))
		require.Equal(t, strconv.Itoa(i), machine.GetLabels()["key"+strconv.Itoa(i)])
	}

	close(sink)
}
