// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

// (DELETE /v2/clusters/{name}/nodes/{nodeId})
func (s *Server) DeleteV2ClustersNameNodesNodeId(ctx context.Context, request api.DeleteV2ClustersNameNodesNodeIdRequestObject) (api.DeleteV2ClustersNameNodesNodeIdResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()
	// first check if we're dealing with a single node cluster
	clusterName := request.Name
	deleteOptions := v1.DeleteOptions{}
	if request.Params.Force != nil && *request.Params.Force {
		// set grace period to 0 for force delete
		deleteOptions = v1.DeleteOptions{GracePeriodSeconds: new(int64)}
		*deleteOptions.GracePeriodSeconds = 0
	}
	if clusterName == "" {
		errMsg := "no cluster name provided"
		slog.Error(errMsg)
		return api.DeleteV2ClustersNameNodesNodeId400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &errMsg}}, nil
	}
	nodeID := request.NodeId
	if nodeID == "" {
		errMsg := "no node id provided"
		slog.Error(errMsg)
		return api.DeleteV2ClustersNameNodesNodeId400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &errMsg}}, nil
	}

	// retrieve the cluster
	cluster, err := s.getCluster(ctx, activeProjectID, clusterName)
	if err != nil {
		errMsg := "failed to retrieve cluster"
		slog.Error(errMsg, "error", err)
		return api.DeleteV2ClustersNameNodesNodeId500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &errMsg}}, nil
	}
	// check for single node
	if len(*cluster.Nodes) == 1 {
		// if we're dealing with a single node cluster, we can delete the capi cluster
		err = deleteCluster(ctx, s, activeProjectID, clusterName, deleteOptions)
		if err != nil {
			errMsg := "failed to delete cluster"
			slog.Error(errMsg, "error", err)
			return api.DeleteV2ClustersNameNodesNodeId500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &errMsg}}, nil
		}
		slog.Info("cluster deleted", "name", clusterName)
		return api.DeleteV2ClustersNameNodesNodeId200Response{}, nil
	}

	slog.Error("multi node clusters are not supported", "name", clusterName)
	return api.DeleteV2ClustersNameNodesNodeId500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: ptr("multi node clusters are not supported")}}, nil
}

func deleteCluster(ctx context.Context, s *Server, activeProjectID, clusterName string, options v1.DeleteOptions) error {
	err := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(activeProjectID).Delete(ctx, clusterName, options)
	if errors.IsNotFound(err) {
		return fmt.Errorf("cluster %s not found in namespace %s", clusterName, activeProjectID)
	}
	return err
}

// nolint: unused
// for future use with multi-node clusters
func scaleDownCluster(ctx context.Context, s *Server, capiCluster *capi.Cluster, nodeID string, options v1.DeleteOptions) error {
	// get the machine associated with the node
	machine, err := fetchMachineFromCluster(ctx, s, capiCluster.Namespace, capiCluster.Name, nodeID)
	if err != nil {
		return err
	}
	// delete the machine
	err = s.k8sclient.Resource(core.MachineResourceSchema).Namespace(capiCluster.Namespace).Delete(ctx, machine.Name, options)
	if errors.IsNotFound(err) {
		return fmt.Errorf("machine %s not found in namespace %s", machine.Name, capiCluster.Namespace)
	}
	// delete the machine binding
	intelMachine, err := fetchIntelMachineBindingFromCluster(ctx, s, capiCluster.Namespace, capiCluster.Name, nodeID)
	if err != nil {
		return err
	}
	err = s.k8sclient.Resource(core.BindingsResourceSchema).Namespace(capiCluster.Namespace).Delete(ctx, intelMachine.Name, options)
	if errors.IsNotFound(err) {
		return fmt.Errorf("could not delete machine binding %s in namespace %s", intelMachine.Name, capiCluster.Namespace)
	}
	// scale down the cluster replicas
	replicas := *capiCluster.Spec.Topology.ControlPlane.Replicas - 1
	capiCluster.Spec.Topology.ControlPlane.Replicas = &replicas
	unstructuredClusterInfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&capiCluster)
	if err != nil {
		return err
	}
	unstructuredCluster := &unstructured.Unstructured{Object: unstructuredClusterInfo}
	_, err = s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(capiCluster.Namespace).Update(ctx, unstructuredCluster, v1.UpdateOptions{})
	return err
}
