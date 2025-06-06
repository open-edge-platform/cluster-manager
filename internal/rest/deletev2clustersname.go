// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"

	"log/slog"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"

	"k8s.io/client-go/dynamic"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (DELETE /v2/clusters/{name})
func (s *Server) DeleteV2ClustersName(ctx context.Context, request api.DeleteV2ClustersNameRequestObject) (api.DeleteV2ClustersNameResponseObject, error) {
	name := request.Name
	activeProjectID := request.Params.Activeprojectid.String()
	if name == "" {
		slog.Debug("Invalid name", "name", name)
		return api.DeleteV2ClustersName400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("no cluster or invalid active project id provided"),
			},
		}, nil
	}

	// Get edge infra managed hosts for the cluster. We should de-authorize them.
	nodes, err := s.GetNodesForCluster(ctx, activeProjectID, s.k8sclient, name)
	if err != nil {
		slog.Error("failed to get nodes for cluster", "namespace", activeProjectID, "name", name, "error", err)
		return api.DeleteV2ClustersName500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("failed to get nodes for cluster"),
			},
		}, nil
	}
	// De-authorize edge infra managed hosts
	for _, node := range nodes {
		err = s.inventory.InvalidateHost(ctx, activeProjectID, node)
		if err != nil {
			slog.Error("failed to de-authorize host", "namespace", activeProjectID, "node", node, "error", err)
			return api.DeleteV2ClustersName500JSONResponse{
				N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
					Message: ptr("failed to de-authorize host"),
				},
			}, nil
		}
	}

	err = s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(activeProjectID).Delete(ctx, name, v1.DeleteOptions{})
	if errors.IsNotFound(err) {
		message := fmt.Sprintf("cluster %s not found in namespace %s", name, activeProjectID)
		return api.DeleteV2ClustersName404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	}

	if err != nil {
		slog.Error("failed to delete cluster", "namespace", activeProjectID, "name", name, "error", err)
		return api.DeleteV2ClustersName500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("failed to delete cluster"),
			},
		}, nil
	}

	slog.Debug("Cluster successfully deleted", "name", name, "activeProjectID", activeProjectID)
	return api.DeleteV2ClustersName204Response{}, nil
}

// GetNodesForCluster retrieves the list of nodes that are managed by edge infrastructure that are associated with a specific cluster
// Note that MachineBindings are used to associate nodes with clusters in the Intel Cluster API provider. If the cluster
// is not managed by the Intel Cluster API provider, this function will return an empty list.
func (s *Server) GetNodesForCluster(ctx context.Context, namespace string, client dynamic.Interface, clusterName string) ([]string, error) {
	var nodes []string

	// Find nodes only if the Infrastructure Provider is Intel
	infraKind, err := fetchInfrastructureRefKind(ctx, s, namespace, clusterName)
	if err != nil {
		slog.Error("failed to fetch infrastructure reference kind", "namespace", namespace, "clusterName", clusterName, "error", err)
		return nil, err
	}
	if infraKind != IntelInfraClusterKind {
		slog.Debug("Skipping node retrieval, cluster is not managed by Intel Cluster API provider", "namespace", namespace, "clusterName", clusterName, "infraKind", infraKind)
		return nodes, nil
	}

	// Use a label selector to filter bindings by clusterName
	opts := v1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", ClusterNameSelectorKey, clusterName),
	}

	// Fetch the filtered list of bindings
	unstructuredBindingsList, err := client.Resource(core.BindingsResourceSchema).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Convert unstructured bindings to IntelMachineBinding objects
	for _, item := range unstructuredBindingsList.Items {
		var binding intelv1alpha1.IntelMachineBinding
		err = convert.FromUnstructured(item, &binding)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, binding.Spec.NodeGUID)
	}

	return nodes, nil
}
