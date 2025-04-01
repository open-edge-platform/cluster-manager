// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package cluster_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	intelInfraProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/internal/cluster"
	"github.com/open-edge-platform/cluster-manager/internal/convert"
	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	k8score "k8s.io/api/core/v1"
	k8sapimachinery "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	_ "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

func WithIntelMachinesMock(t *testing.T, namespace, clusterName string, machines []capi.Machine, intelMachines []intelInfraProvider.IntelMachine) func(*k8s.Client) {
	labelsSelector := fmt.Sprintf("cluster.x-k8s.io/cluster-name=%v", clusterName)

	return func(cli *k8s.Client) {
		// machines
		um, err := convert.ToUnstructuredList(machines)
		require.NoError(t, err)

		machinesResource := k8s.NewMockResourceInterface(t)
		machinesResource.EXPECT().List(mock.Anything, k8sapimachinery.ListOptions{LabelSelector: labelsSelector}).Return(um, nil)

		nMachinesResource := k8s.NewMockNamespaceableResourceInterface(t)
		nMachinesResource.EXPECT().Namespace(namespace).Return(machinesResource)

		// dynamic client
		c := k8s.NewMockInterface(t)
		c.EXPECT().Resource(core.MachineResourceSchema).Return(nMachinesResource)

		// intel machines
		for _, im := range intelMachines {
			um, err := convert.ToUnstructured(im)
			require.NoError(t, err)

			intelMachineResource := k8s.NewMockResourceInterface(t)
			intelMachineResource.EXPECT().Get(mock.Anything, im.ObjectMeta.Name, k8sapimachinery.GetOptions{}).Return(um, nil)

			nIntelMachineResource := k8s.NewMockNamespaceableResourceInterface(t)
			nIntelMachineResource.EXPECT().Namespace(namespace).Return(intelMachineResource)

			c.EXPECT().Resource(k8s.IntelMachineResourceSchema).Return(nIntelMachineResource)
		}

		cli.Dyn = c
	}
}

func TestNodes(t *testing.T) {
	// expected cluster
	namespace := "test-namespace"
	clusterName := "test-cluster"
	c := &capi.Cluster{
		ObjectMeta: k8sapimachinery.ObjectMeta{Name: clusterName, Namespace: namespace},
		Spec:       capi.ClusterSpec{Topology: &capi.Topology{Class: "test-template", ControlPlane: capi.ControlPlaneTopology{Replicas: convert.Ptr(int32(1))}}},
	}

	// expected machine
	machineName := "test-machine"
	machines := []capi.Machine{
		{
			ObjectMeta: k8sapimachinery.ObjectMeta{Name: machineName, Namespace: namespace},
			Spec:       capi.MachineSpec{InfrastructureRef: k8score.ObjectReference{Name: "test-intel-machine", Kind: "IntelMachine"}},
		},
	}

	// expected Intel Machine
	intelMachineName := "test-intel-machine"
	intelMachineNameId := "host-abcd"
	intelMachines := []intelInfraProvider.IntelMachine{{
		ObjectMeta: k8sapimachinery.ObjectMeta{Name: intelMachineName, Namespace: namespace, Annotations: map[string]string{cluster.HostIdAnnotationKey: intelMachineNameId}}}}

	// expected output node
	condition := api.STATUSCONDITIONUNKNOWN
	status := api.StatusInfo{Condition: &condition, Reason: convert.Ptr("Unknown"), Timestamp: nil}
	expectedNode := []api.NodeInfo{{Id: convert.Ptr(intelMachineNameId), Role: convert.Ptr("all"), Status: &status}}

	// mock k8s client
	cli, err := k8s.New(WithIntelMachinesMock(t, namespace, clusterName, machines, intelMachines))
	assert.NoError(t, err)

	// test
	nodes, err := cluster.Nodes(context.Background(), cli, c)
	assert.NoError(t, err)
	if diff := cmp.Diff(nodes, expectedNode); diff != "" {
		t.Errorf("Nodes() mismatch (-want +got):\n%s", diff)
	}
}

func TestTemplate(t *testing.T) {
	expectedTemplate := "test-template"
	c := &capi.Cluster{Spec: capi.ClusterSpec{Topology: &capi.Topology{Class: expectedTemplate}}}

	template := cluster.Template(c)
	if template != expectedTemplate {
		t.Errorf("Expected %s, got %s", expectedTemplate, template)
	}
}
