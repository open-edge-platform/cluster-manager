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
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestHostCreatedHandle(t *testing.T) {
	sink := events.Sink()

	for i := 0; i < 10; i++ {
		event := events.HostCreated{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
		}
		sink <- event
	}

	close(sink)
}

func TestHostDeleteHandle(t *testing.T) {
	sink := events.Sink()

	for i := 0; i < 10; i++ {
		event := events.HostDeletedEvent{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
		}
		sink <- event
	}

	close(sink)
}

func TestHostUpdateHandle(t *testing.T) {
	sink := events.Sink()

	// create a new mocked k8s client
	mockedk8sclient := k8s.NewMockInterface(t)

	// mocked Machine object
	activeProjectID := "64e797f6-db23-445e-b606-4228d4f1c2bd"
	nodeId := "64e797f6-db22-445e-b606-4228d4f1c2bd"
	machine, err := convert.ToUnstructured(capi.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "example-machine", Namespace: activeProjectID},
		TypeMeta:   metav1.TypeMeta{APIVersion: "cluster.x-k8s.io/v1beta1", Kind: "Machine"},
		Spec:       capi.MachineSpec{InfrastructureRef: v1.ObjectReference{Name: "example-infrastructure", Kind: "IntelMachine", Namespace: activeProjectID}},
		Status:     capi.MachineStatus{NodeRef: &v1.ObjectReference{UID: types.UID(nodeId)}}})
	require.Nil(t, err)

	// create a new mocked machine resource
	machineResource := k8s.NewMockResourceInterface(t)
	machineResource.EXPECT().List(mock.Anything, mock.Anything).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*machine}}, nil).Maybe()
	nsMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsMachineResource.EXPECT().Namespace(activeProjectID).Return(machineResource).Maybe()
	mockedk8sclient.EXPECT().Resource(core.MachineResourceSchema).Return(nsMachineResource).Maybe()

	cli, err := k8s.New(k8s.WithDynamicClient(mockedk8sclient))
	require.Nil(t, err)

	for i := 0; i < 1; i++ {
		event := events.HostUpdate{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
			Labels:    map[string]string{"key": "value"},
			K8scli:    cli,
		}
		sink <- event
	}

	close(sink)
}
