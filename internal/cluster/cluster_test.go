// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package cluster_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	intelInfraProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/cluster"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	k8score "k8s.io/api/core/v1"
	k8sapimachinery "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	_ "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

func WithIntelMachinesMock(t *testing.T, namespace, clusterName string, machines []capi.Machine, intelMachines []intelInfraProvider.IntelMachine) k8s.Client {
	mockClient := k8s.NewMockClient(t)

	// Mock GetMachines call
	mockClient.On("GetMachines", mock.Anything, namespace, clusterName).Return(machines, nil)

	// Mock IntelMachine calls for each machine
	for _, im := range intelMachines {
		mockClient.On("IntelMachine", mock.Anything, namespace, im.ObjectMeta.Name).Return(im, nil)
	}

	return mockClient
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
	cli := WithIntelMachinesMock(t, namespace, clusterName, machines, intelMachines)
	assert.NotNil(t, cli)

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
