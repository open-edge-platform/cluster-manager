// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package cluster

import (
	"context"
	"fmt"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// HostIdAnnotationKey is the key used to store the host ID in the annotations of the provider machine.
	HostIdAnnotationKey = "intelmachine.infrastructure.cluster.x-k8s.io/host-id"
	roleAll             = "all"
)

// Nodes returns the list of nodes in the cluster.
func Nodes(ctx context.Context, cli k8s.Client, cluster *capi.Cluster) ([]api.NodeInfo, error) {
	nodes := []api.NodeInfo{}

	machines, err := cli.GetMachines(ctx, cluster.Namespace, cluster.Name)
	if err != nil {
		return nodes, err
	}

	for _, m := range machines {
		id, err := getNodeId(ctx, cli, m)
		if err != nil {
			return nodes, err
		}
		role := nodeRole(m)
		status := getNodeStatus(m)
		nodes = append(nodes, api.NodeInfo{Id: &id, Role: &role, Status: &status})
	}

	return nodes, nil
}

// Template returns the cluster template name.
func Template(c *capi.Cluster) string {
	if c == nil || c.Spec.Topology == nil {
		return ""
	}

	return c.Spec.Topology.Class
}

func getNodeStatus(machine capi.Machine) api.StatusInfo {
	translate := map[capi.MachinePhase]api.StatusInfoCondition{
		// MachinePhasePending is the first state a Machine is assigned by Cluster API Machine controller after being created.
		capi.MachinePhasePending: api.STATUSCONDITIONPROVISIONING,
		// MachinePhaseProvisioning is the state when the Machine infrastructure is being created.
		capi.MachinePhaseProvisioning: api.STATUSCONDITIONPROVISIONING,
		// MachinePhaseProvisioned is the state when its infrastructure has been created and configured.
		capi.MachinePhaseProvisioned: api.STATUSCONDITIONPROVISIONING,
		// MachinePhaseRunning is the Machine state when it has become a Kubernetes Node in a Ready state.
		capi.MachinePhaseRunning: api.STATUSCONDITIONREADY,
		// MachinePhaseDeleting is the Machine state when a delete request has been sent to the API Server,
		capi.MachinePhaseDeleting: api.STATUSCONDITIONREMOVING,
		// MachinePhaseDeleted is the Machine state when the object and the related infrastructure is deleted and ready to be garbage collected by the API Server.
		capi.MachinePhaseDeleted: api.STATUSCONDITIONNOTREADY,
		// MachinePhaseFailed is the Machine state when the system might require user intervention.
		capi.MachinePhaseFailed: api.STATUSCONDITIONUNKNOWN,
		// MachinePhaseUnknown is returned if the Machine state cannot be determined.
		capi.MachinePhaseUnknown: api.STATUSCONDITIONUNKNOWN,
	}

	status := api.StatusInfo{}
	if machine.Status.LastUpdated != nil {
		status.Timestamp = convert.Ptr(machine.Status.LastUpdated.String())
	}
	phase := machine.Status.GetTypedPhase()
	status.Condition = convert.Ptr(translate[phase])
	status.Reason = convert.Ptr(string(phase))
	return status
}

func getNodeId(ctx context.Context, cli k8s.Client, machine capi.Machine) (string, error) {
	providerMachineName := machine.Spec.InfrastructureRef.Name
	providerMachineKind := machine.Spec.InfrastructureRef.Kind
	switch providerMachineKind {
	case "IntelMachine":
		providerMachine, err := cli.IntelMachine(ctx, machine.Namespace, providerMachineName)
		if err != nil {
			return "", err
		}
		return providerMachine.Annotations[HostIdAnnotationKey], nil
	case "DockerMachine":
		providerMachine, err := cli.DockerMachine(ctx, machine.Namespace, providerMachineName)
		if err != nil {
			return "", err
		}
		return providerMachine.Annotations[HostIdAnnotationKey], nil
	}
	return "", fmt.Errorf("unsupported provider machine kind %s", providerMachineKind)
}

// TODO: add multi-node support
func nodeRole(machine capi.Machine) string {
	return roleAll
}
