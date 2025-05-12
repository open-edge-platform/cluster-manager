// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/open-edge-platform/cluster-manager/v2/internal/cluster"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (GET /v2/clusters/{name})
func (s *Server) GetV2ClustersName(ctx context.Context, request api.GetV2ClustersNameRequestObject) (api.GetV2ClustersNameResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()

	// TODO: we have a middleware for this, but we can't use it since the tests are calling this function directly
	// we need to refactor the tests to invoke this function indirectly via handler.ServeHTTP(rr, req)
	if activeProjectID == "" || activeProjectID == "00000000-0000-0000-0000-000000000000" {
		return api.GetV2ClustersName400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("no active project id provided"),
			},
		}, nil
	}

	name := request.Name
	if name == "" {
		return api.GetV2ClustersName400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("cluster name is required"),
			},
		}, nil
	}

	// Validate the name using a regex pattern
	validNamePattern := `^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$`
	matched, err := regexp.MatchString(validNamePattern, name)
	if err != nil || !matched {
		// nolint: nilerr
		return api.GetV2ClustersName400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("invalid cluster name format"),
			},
		}, nil
	}

	cluster, err := s.getCluster(ctx, activeProjectID, name)
	if err != nil {
		if errors.Unwrap(err) == k8s.ErrClusterNotFound {
			return api.GetV2ClustersName404JSONResponse{
				N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{
					Message: ptr(err.Error()),
				},
			}, nil
		}
		slog.Error("failed to get cluster", "name", name, "error", err)
		return api.GetV2ClustersName500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	return api.GetV2ClustersName200JSONResponse(cluster), nil
}

// getCluster retrieves a cluster from the k8s client
func (s *Server) getCluster(ctx context.Context, activeProjectID, name string) (api.ClusterDetailInfo, error) {
	namespace := activeProjectID

	cli, err := k8s.New(k8s.WithDynamicClient(s.k8sclient))
	if err != nil {
		slog.Error("failed to create k8s client", "error", err)
		return api.ClusterDetailInfo{}, fmt.Errorf("failed to create k8s client, err: %w", err)
	}

	capiCluster, err := cli.GetCluster(ctx, namespace, name)
	if err != nil {
		slog.Error("failed to get cluster", "name", name, "error", err)
		return api.ClusterDetailInfo{}, fmt.Errorf("failed to get cluster, err: %w", err)
	}
	if capiCluster.Name == "" {
		return api.ClusterDetailInfo{}, errors.New("missing cluster name")
	}

	// get machines associated with the cluster
	machines, err := fetchMachinesList(ctx, s, namespace, capiCluster.Name)
	if err != nil {
		// do we need to return error here?
		slog.Error("failed to fetch machines for cluster", "cluster", capiCluster.Name, "error", err)
	}

	labels := labels.UserLabels(capiCluster.Labels)
	unstrucutreLabels := convert.MapStringToAny(labels)

	nodes, err := cluster.Nodes(ctx, cli, capiCluster)
	if err != nil {
		slog.Error("failed to get nodes", "cluster", capiCluster.Name, "error", err)
		return api.ClusterDetailInfo{}, fmt.Errorf("failed to get nodes, err: %w", err)
	}

	template := cluster.Template(capiCluster)
	lp, errs := getClusterLifecyclePhase(capiCluster)
	if len(errs) > 0 {
		slog.Debug("errors while building cluster lifecycle phase", "cluster", capiCluster.Name, "errors", errs)
	}

	clusterDetailInfo := api.ClusterDetailInfo{
		Name:                &capiCluster.Name,
		ProviderStatus:      getProviderStatus(capiCluster),
		Labels:              &unstrucutreLabels,
		KubernetesVersion:   getKubernetesVersion(capiCluster),
		LifecyclePhase:      lp,
		ControlPlaneReady:   getControlPlaneReady(capiCluster),
		InfrastructureReady: getInfrastructureReady(capiCluster),
		NodeHealth:          getNodeHealth(capiCluster, machines),
		Nodes:               &nodes,
		Template:            &template,
	}

	if err := validateClusterDetail(clusterDetailInfo); err != nil {
		slog.Error("failed to validate cluster detail", "cluster", capiCluster.Name, "error", err)
		return api.ClusterDetailInfo{}, fmt.Errorf("failed to validate cluster detail, err: %w", err)
	}

	return clusterDetailInfo, nil
}
