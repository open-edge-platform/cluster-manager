// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestHostCreatedHandle(t *testing.T) {
	// Setup
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Test multiple host created events
	for i := 0; i < 3; i++ {
		event := createHostCreatedEvent(i)
		sink <- event
	}

	close(sink)
}

func TestHostDeleteHandle(t *testing.T) {
	// Setup
	ctx := context.Background()
	sink := events.NewSink(ctx)
	out := make(chan error, 1)

	// Test multiple host delete events
	for i := 0; i < 3; i++ {
		event := createHostDeletedEvent(i, out)
		sink <- event
		require.NoError(t, <-out) // wait for the event to be handled
	}

	close(sink)
}

func TestHostUpdateHandle(t *testing.T) {
	// Setup
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Test data
	projectID := "64e797f6-db23-445e-b606-4228d4f1c2bd"
	hostId := "host-12345"
	labels := map[string]string{"key": "value"}

	// Setup mock k8s client and resources
	cli, machine := setupMockK8sClient(t, projectID, hostId)

	// Test multiple host update events
	out := make(chan error, 1)
	for i := 0; i < 3; i++ {
		event := createHostUpdatedEvent(hostId, projectID, labels, out, cli)
		sink <- event
		require.NoError(t, <-out) // wait for the event to be handled
	}

	// Verify the results
	require.Equal(t, 1, len(machine.GetLabels()))
	require.Equal(t, "value", machine.GetLabels()["key"])

	close(sink)
}

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

// Helper functions to create test events and mocks

func createHostCreatedEvent(index int) events.HostCreated {
	return events.HostCreated{
		HostEventBase: events.HostEventBase{
			HostId:    "test-host-id-" + strconv.Itoa(index),
			ProjectId: "test-project-id-" + strconv.Itoa(index),
		},
	}
}

func createHostDeletedEvent(index int, out chan<- error) events.HostDeleted {
	return events.HostDeleted{
		HostEventBase: events.HostEventBase{
			EventBase: events.EventBase{Out: out},
			HostId:    "test-host-id-" + strconv.Itoa(index),
			ProjectId: "test-project-id-" + strconv.Itoa(index),
		},
	}
}

func createHostUpdatedEvent(hostId, projectID string, labels map[string]string, out chan<- error, cli *k8s.Client) events.HostUpdated {
	return events.HostUpdated{
		HostEventBase: events.HostEventBase{
			EventBase: events.EventBase{Out: out},
			HostId:    hostId,
			ProjectId: projectID,
		},
		Labels: labels,
		K8scli: cli,
	}
}

func setupMockK8sClient(t *testing.T, projectID, hostId string) (*k8s.Client, *unstructured.Unstructured) {
	// Create a new mocked k8s client
	mockedk8sclient := k8s.NewMockInterface(t)

	// Mocked Machine object
	machine, err := convert.ToUnstructured(capi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-machine",
			Namespace: projectID,
			Labels:    map[string]string{},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1beta1",
			Kind:       "Machine",
		},
		Spec: capi.MachineSpec{
			InfrastructureRef: v1.ObjectReference{
				Name:      "example-infrastructure",
				Kind:      "IntelMachine",
				Namespace: projectID,
			},
			ProviderID: &hostId,
		},
	})
	require.Nil(t, err)

	// Create a new mocked machine resource
	machineResource := k8s.NewMockResourceInterface(t)
	machineResource.EXPECT().List(mock.Anything, mock.Anything).Return(
		&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil)
	machineResource.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(machine, nil)
	machineResource.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(machine, nil)

	nsMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsMachineResource.EXPECT().Namespace(projectID).Return(machineResource)
	mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsMachineResource)

	cli, err := k8s.New(k8s.WithDynamicClient(mockedk8sclient))
	require.Nil(t, err)

	return cli, machine
}
