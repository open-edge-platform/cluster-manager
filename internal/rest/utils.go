// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

type ContextKey string

const (
	ClusterNameSelectorKey  = "cluster.x-k8s.io/cluster-name"
	IntelInfraClusterKind   = "IntelCluster"
	IntelMachineBindingKind = "IntelMachineBinding"
)

func validateClusterDetail(clusterDetail api.ClusterDetailInfo) error {
	if clusterDetail.Name == nil || *clusterDetail.Name == "" {
		return errors.New("missing or invalid cluster name")
	}
	if clusterDetail.ProviderStatus == nil || clusterDetail.ProviderStatus.Indicator == nil || *clusterDetail.ProviderStatus.Indicator == "" {
		return errors.New("missing or invalid provider status")
	}

	if clusterDetail.Labels == nil {
		return errors.New("missing or invalid labels")
	}

	if clusterDetail.KubernetesVersion == nil || *clusterDetail.KubernetesVersion == "" {
		return errors.New("missing or invalid Kubernetes version")
	}
	if clusterDetail.LifecyclePhase == nil || clusterDetail.LifecyclePhase.Indicator == nil || *clusterDetail.LifecyclePhase.Indicator == "" {
		return errors.New("missing or invalid lifecycle phase")
	}
	if clusterDetail.NodeHealth == nil || clusterDetail.NodeHealth.Indicator == nil || *clusterDetail.NodeHealth.Indicator == "" {
		return errors.New("missing or invalid node health")
	}
	if clusterDetail.Nodes == nil || len(*clusterDetail.Nodes) == 0 {
		return errors.New("missing or invalid nodes")
	}
	if clusterDetail.Template == nil || *clusterDetail.Template == "" {
		return errors.New("missing or invalid template")
	}
	return nil
}

func getComponentReady(cluster *capi.Cluster, conditionType capi.ConditionType, readyMessage, notReadyMessage, unknownMessage, notFoundMessage string) *api.GenericStatus {
	status := &api.GenericStatus{
		Indicator: new(api.StatusIndicator),
		Message:   new(string),
		Timestamp: new(uint64),
	}

	if len(cluster.Status.Conditions) == 0 {
		*status.Indicator = api.STATUSINDICATIONUNSPECIFIED
		*status.Message = notFoundMessage
		*status.Timestamp = 0
		return status
	}

	var componentCondition *capi.Condition
	for _, cond := range cluster.Status.Conditions {
		if cond.Type == conditionType {
			componentCondition = &cond
			break
		}
	}

	if componentCondition == nil {
		*status.Indicator = api.STATUSINDICATIONUNSPECIFIED
		*status.Message = notFoundMessage
		*status.Timestamp = 0
		return status
	}

	*status.Timestamp = uint64(componentCondition.LastTransitionTime.UTC().Unix())

	switch componentCondition.Status {
	case corev1.ConditionTrue:
		*status.Indicator = api.STATUSINDICATIONIDLE
		*status.Message = readyMessage
	case corev1.ConditionFalse:
		*status.Indicator = api.STATUSINDICATIONINPROGRESS
		*status.Message = notReadyMessage
	default:
		*status.Indicator = api.STATUSINDICATIONERROR
		*status.Message = unknownMessage
	}
	if componentCondition.Status == corev1.ConditionFalse {
		if componentCondition.Reason == "WaitingForRKE2Server" || componentCondition.Reason == "WaitingForKThreesServer" {
			*status.Message = fmt.Sprintf("%s;%s", *status.Message, "waiting for control plane provider to indicate the control plane has been initialized")
		} else {
			*status.Message = fmt.Sprintf("%s;%s", *status.Message, componentCondition.Reason)
		}

	}
	return status
}

func getProviderStatus(cluster *capi.Cluster) *api.GenericStatus {
	return getComponentReady(
		cluster,
		capi.ReadyCondition,
		"ready",
		"not ready",
		"unknown",
		"condition not found",
	)
}

func getControlPlaneReady(cluster *capi.Cluster) *api.GenericStatus {
	return getComponentReady(
		cluster,
		capi.ControlPlaneReadyCondition,
		"ready",
		"not ready",
		"status is unknown",
		"condition not found",
	)
}

func getInfrastructureReady(cluster *capi.Cluster) *api.GenericStatus {
	return getComponentReady(
		cluster,
		capi.InfrastructureReadyCondition,
		"ready",
		"not ready",
		"unknown",
		"condition not found",
	)
}

func getKubernetesVersion(cluster *capi.Cluster) *string {
	if cluster.Spec.Topology != nil && cluster.Spec.Topology.Version != "" {
		return &cluster.Spec.Topology.Version
	}
	return nil
}

func getClusterLifecyclePhase(cluster *capi.Cluster) (*api.GenericStatus, []error) {
	status := api.GenericStatus{
		Indicator: new(api.StatusIndicator),
		Message:   new(string),
		Timestamp: new(uint64),
	}
	var errorReasons []error
	if len(cluster.Status.Conditions) == 0 {
		*status.Indicator = api.STATUSINDICATIONUNSPECIFIED
		*status.Message = "Condition not found"
		*status.Timestamp = 0
		return &status, errorReasons
	}
	// ClusterPhase is a string representation of a Cluster Phase.
	// It is a high-level indicator of the status of the Cluster
	// as it is provisioned, from the API user’s perspective.
	// The value should not be interpreted by any software components
	// as a reliable indication of the actual state of the Cluster
	// Phase represents the current phase of cluster actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/cluster_phase_types.go#L19
	switch cluster.Status.Phase {
	case string(capi.ClusterPhasePending):
		*status.Indicator = api.STATUSINDICATIONINPROGRESS
		*status.Message = "pending"
	case string(capi.ClusterPhaseProvisioning):
		*status.Indicator = api.STATUSINDICATIONINPROGRESS
		*status.Message = "provisioning"
	case string(capi.ClusterPhaseDeleting):
		*status.Indicator = api.STATUSINDICATIONINPROGRESS
		*status.Message = "deleting"
	case string(capi.ClusterPhaseProvisioned):
		// Note: in provisioned phase parts of the control plane or worker machines might be still provisioning
		// https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/cluster_phase_types.go#L42
		if *getProviderStatus(cluster).Indicator != api.STATUSINDICATIONIDLE {
			*status.Indicator = api.STATUSINDICATIONINPROGRESS
			*status.Message = "provisioned"
		} else {
			*status.Indicator = api.STATUSINDICATIONIDLE
			*status.Message = "active"
		}
	case string(capi.ClusterPhaseFailed):
		*status.Indicator = api.STATUSINDICATIONERROR
		*status.Message = "failed"
		for _, cond := range cluster.Status.Conditions {
			if cond.Status == corev1.ConditionFalse {
				errorReasons = append(errorReasons, fmt.Errorf("%s: %s", cond.Reason, cond.Message))
			}
		}
	case string(capi.ClusterPhaseUnknown):
		*status.Indicator = api.STATUSINDICATIONUNSPECIFIED
		*status.Message = "unknown"
	default:
		return nil, errorReasons
	}

	*status.Timestamp = uint64(cluster.Status.Conditions[0].LastTransitionTime.UTC().Unix())
	return &status, errorReasons
}

// getNodeHealth evaluates the health status of each machine in the cluster, i.e.,  machine ~ node
func getNodeHealth(cluster *capi.Cluster, machines []unstructured.Unstructured) *api.GenericStatus {
	status := &api.GenericStatus{
		Indicator: new(api.StatusIndicator),
		Message:   new(string),
		Timestamp: new(uint64),
	}

	allHealthy := true
	noneHealthy := true
	inProgress := false
	var messages []string
	var machineMessage []string
	machinesRunning := 0

	appendMessage := func(msg string) {
		messages = append(messages, msg)
	}

	totalMachines := len(machines)
	for _, machine := range machines {
		var machineObj capi.Machine
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(machine.Object, &machineObj); err != nil {
			allHealthy = false
			appendMessage("failed to convert machine")
			continue
		}

		for _, condition := range machineObj.Status.Conditions {
			switch condition.Type {
			case capi.MachineHealthCheckSucceededCondition, capi.MachineNodeHealthyCondition:
				if condition.Status == corev1.ConditionFalse {
					allHealthy = false
					inProgress = true
					if condition.Reason == "WaitingForNodeRef" {
						appendMessage(fmt.Sprintf("%s: %s", machineObj.Name, "waiting for node information to be populated"))
					} else {
						appendMessage(fmt.Sprintf("%s: %s", machineObj.Name, condition.Reason))
					}
					machineMessage = append(machineMessage, fmt.Sprintf("%s: %s", machineObj.Name, condition.Message))
				} else {
					allHealthy = true
				}
			}
		}

		// check if the machine is in the running phase
		if capi.MachinePhase(machineObj.Status.Phase) == capi.MachinePhaseRunning {
			inProgress = false
			machinesRunning++
			continue
		}
		if capi.MachinePhase(machineObj.Status.Phase) == capi.MachinePhaseProvisioning {
			inProgress = true
			appendMessage(fmt.Sprintf("%s: %s", machineObj.Name, capi.MachinePhaseProvisioning))
			continue
		}
		// collecting info on machine phase if not running
		if noneHealthy {
			machineMessage = append(machineMessage, fmt.Sprintf("MachinePhase %v", capi.MachinePhase(machineObj.Status.Phase)))
		}
	}

	if inProgress {
		*status.Indicator = api.STATUSINDICATIONINPROGRESS
		*status.Message = fmt.Sprintf("node(s) health unknown (%v/%v);%s", machinesRunning, totalMachines, messages)
	} else if allHealthy && machinesRunning == totalMachines {
		*status.Indicator = api.STATUSINDICATIONIDLE
		*status.Message = "nodes are healthy"
	} else if noneHealthy {
		*status.Indicator = api.STATUSINDICATIONERROR
		*status.Message = fmt.Sprintf("nodes are unhealthy (%v/%v);%s", machinesRunning, totalMachines, machineMessage)
	}

	if len(cluster.Status.Conditions) > 0 {
		*status.Timestamp = uint64(cluster.Status.Conditions[0].LastTransitionTime.UTC().Unix())
	} else {
		*status.Timestamp = 0
	}

	return status
}

// getClusterMachines filters the machines by cluster name
func getClusterMachines(machines []unstructured.Unstructured, name string) []unstructured.Unstructured {
	var filteredMachines []unstructured.Unstructured
	for _, machine := range machines {
		if machine.GetLabels()[ClusterNameSelectorKey] == name {
			filteredMachines = append(filteredMachines, machine)
		}
	}
	return filteredMachines
}

func ptr[T any](v T) *T {
	return &v
}

func fetchClustersList(ctx context.Context, s *Server, namespace string) ([]unstructured.Unstructured, error) {
	unstructuredClusterList, err := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return unstructuredClusterList.Items, nil
}

// nolint: unused
func fetchClusterObject(ctx context.Context, s *Server, namespace string, name string) (*capi.Cluster, error) {
	unstructuredCluster, err := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if unstructuredCluster == nil {
		return nil, errors.New("cluster not found")
	}
	if err != nil {
		return nil, err
	}
	cluster := capi.Cluster{}
	err = convert.FromUnstructured(*unstructuredCluster, cluster)
	return &cluster, err
}

// nolint: unused
func fetchMachineFromCluster(ctx context.Context, s *Server, namespace string, clusterName string, nodeID string) (*capi.Machine, error) {
	unstructuredMachines, err := fetchMachinesList(ctx, s, namespace, clusterName)
	if err != nil {
		return nil, err
	}
	for _, unstructuredMachine := range unstructuredMachines {
		machine := &capi.Machine{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredMachine.Object, machine); err != nil {
			continue
		}
		if machine.Status.NodeRef != nil && string(machine.Status.NodeRef.UID) == nodeID {
			return machine, nil
		}
	}
	return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
}

// nolint: unused
func fetchIntelMachineBindingFromCluster(ctx context.Context, s *Server, namespace string, clusterName string, nodeID string) (*intelv1alpha1.IntelMachineBinding, error) {
	// first get the machine using the nodeID
	unstructuredMachines, err := s.k8sclient.Resource(core.MachineResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: ClusterNameSelectorKey + "=" + clusterName,
	})
	if unstructuredMachines == nil || len(unstructuredMachines.Items) == 0 {
		return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
	}
	if err != nil {
		return nil, err
	}
	var targetMachine *capi.Machine
	machines, err := convertUnsructuredtoMachine(unstructuredMachines)
	if err != nil {
		return nil, err
	}
	for _, machine := range machines {
		nodeRef := machine.Status.NodeRef
		if nodeRef != nil && string(nodeRef.UID) == nodeID {
			targetMachine = machine
			break
		}
	}
	if targetMachine == nil {
		return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
	}
	// get the machine bindings for the cluster
	unstructuredMachineList, err := s.k8sclient.Resource(core.BindingsResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: ClusterNameSelectorKey + "=" + clusterName,
	})
	if err != nil {
		return nil, err
	}
	for _, item := range unstructuredMachineList.Items {
		machineBinding := &intelv1alpha1.IntelMachineBinding{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, machineBinding); err != nil {
			continue
		}
		// use the machine reference to find the machine binding
		for _, reference := range machineBinding.ObjectMeta.OwnerReferences {
			if string(reference.UID) == string(targetMachine.Status.NodeRef.UID) {
				return machineBinding, nil
			}
		}
	}
	return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
}

func fetchMachine(ctx context.Context, s *Server, namespace string, clusterName string, nodeID string) (*capi.Machine, error) {
	unstructuredMachines, err := s.k8sclient.Resource(core.MachineResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: ClusterNameSelectorKey + "=" + clusterName,
	})
	if unstructuredMachines == nil || len(unstructuredMachines.Items) == 0 {
		return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
	}
	if err != nil {
		return nil, err
	}
	machines, err := convertUnsructuredtoMachine(unstructuredMachines)
	if err != nil {
		return nil, err
	}
	for _, machine := range machines {
		nodeRef := machine.Status.NodeRef
		if nodeRef != nil && string(nodeRef.UID) == nodeID {
			return machine, nil
		}
	}
	return nil, fmt.Errorf("machine not found for node ID %s", nodeID)
}

func fetchMachinesList(ctx context.Context, s *Server, namespace string, clusterName string) ([]unstructured.Unstructured, error) {
	unstructuredMachineList, err := s.k8sclient.Resource(core.MachineResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: ClusterNameSelectorKey + "=" + clusterName,
	})
	if err != nil {
		return nil, err
	}
	return unstructuredMachineList.Items, nil
}

func fetchAllMachinesList(ctx context.Context, s *Server, namespace string) ([]unstructured.Unstructured, error) {
	unstructuredMachineList, err := s.k8sclient.Resource(core.MachineResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return unstructuredMachineList.Items, nil
}

func fetchInfrastructureRefKind(ctx context.Context, s *Server, namespace string, clusterName string) (string, error) {
	unstructuredCluster, err := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(namespace).Get(ctx, clusterName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	cluster := &capi.Cluster{}
	err = convert.FromUnstructured(*unstructuredCluster, cluster)
	if err != nil {
		return "", err
	}
	if cluster.Spec.InfrastructureRef == nil {
		return "", fmt.Errorf("infrastructure reference not found for cluster %s", clusterName)
	}
	return cluster.Spec.InfrastructureRef.Kind, nil
}
