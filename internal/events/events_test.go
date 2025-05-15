// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events_test

import (
	"strconv"
	"testing"

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
	sink := events.Sink()

	for i := 0; i < 3; i++ {
		event := events.HostCreated{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
			Error:     nil,
		}
		sink <- event
	}

	close(sink)
}

func TestHostDeleteHandle(t *testing.T) {
	sink := events.Sink()
	out := make(chan error, 1)

	for i := 0; i < 3; i++ {
		event := events.HostDeletedEvent{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
			Error:     out,
		}
		sink <- event
		require.NoError(t, <-out) // wait for the event to be handled
	}
	close(sink)
}

func TestHostUpdateHandle(t *testing.T) {
	sink := events.Sink()

	// create a new mocked k8s client
	mockedk8sclient := k8s.NewMockInterface(t)

	// mocked Machine object
	projectID := "64e797f6-db23-445e-b606-4228d4f1c2bd"
	hostId := "host-12345"
	machine, err := convert.ToUnstructured(capi.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "example-machine", Namespace: projectID, Labels: map[string]string{}},
		TypeMeta:   metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"},
		Spec:       capi.MachineSpec{InfrastructureRef: v1.ObjectReference{Name: "example-infrastructure", Kind: "IntelMachine", Namespace: projectID}, ProviderID: &hostId},
	})
	require.Nil(t, err)

	// create a new mocked machine resource
	machineResource := k8s.NewMockResourceInterface(t)
	machineResource.EXPECT().List(mock.Anything, mock.Anything).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil)
	machineResource.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(machine, nil)
	machineResource.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(machine, nil)

	nsMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsMachineResource.EXPECT().Namespace(projectID).Return(machineResource)
	mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsMachineResource)

	cli, err := k8s.New(k8s.WithDynamicClient(mockedk8sclient))
	require.Nil(t, err)

	out := make(chan error, 1)
	for i := 0; i < 3; i++ {
		event := events.HostUpdate{
			HostId:    hostId,
			ProjectId: projectID,
			Labels:    map[string]string{"key": "value"},
			K8scli:    cli,
			Error:     out,
		}
		sink <- event
		require.NoError(t, <-out) // wait for the event to be handled
	}

	require.Equal(t, 1, len(machine.GetLabels()))
	require.Equal(t, "value", machine.GetLabels()["key"])
	close(sink)
}
