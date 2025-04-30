// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	. "github.com/open-edge-platform/cluster-manager/v2/internal/pagination"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

const MaxClusters = math.MaxInt32 // Maximum value for int32

// (GET /v2/clusters)
func (s *Server) GetV2Clusters(ctx context.Context, request api.GetV2ClustersRequestObject) (api.GetV2ClustersResponseObject, error) {
	pageSize, offset, orderBy, filter, err := ValidateParams(request.Params)
	if err != nil {
		slog.Error("failed to validate parameters", "pageSize", pageSize, "offset", offset, "orderBy", orderBy, "filter", filter, "error", err)
		return badRequestGetClustersResponse(err.Error()), nil
	}

	namespace := request.Params.Activeprojectid.String()
	clusters, err := s.getClusters(ctx, namespace, orderBy, filter)
	if err != nil {
		slog.Error("failed to get clusters", "namespace", namespace, "filter", filter, "order", orderBy, "error", err)
		return internalServerErrorGetClustersResponse("failed to retrieve clusters"), nil
	}

	if len(*clusters) == 0 {
		return api.GetV2Clusters200JSONResponse{
			Clusters:      clusters,
			TotalElements: 0,
		}, nil
	}

	paginatedClusters, err := PaginateItems(*clusters, *pageSize, *offset)
	if err != nil {
		slog.Error("failed to paginate clusters", "namespace", namespace, "pageSize", pageSize, "offset", offset, "error", err)
		return internalServerErrorGetClustersResponse(err.Error()), nil
	}

	if len(*paginatedClusters) > MaxClusters {
		slog.Error("number of clusters exceeds the maximum allowed", "namespace", namespace, "count", len(*paginatedClusters))
		return badRequestGetClustersResponse("number of clusters exceeds the maximum allowed"), nil
	}

	slog.Info("Clusters state read", "namespace", namespace, "count", len(*clusters))

	return api.GetV2Clusters200JSONResponse{
		Clusters:      paginatedClusters,
		TotalElements: int32(len(*clusters)),
	}, nil
}

func badRequestGetClustersResponse(message string) api.GetV2Clusters400JSONResponse {
	return api.GetV2Clusters400JSONResponse{
		N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
			Message: ptr(message),
		},
	}
}

func internalServerErrorGetClustersResponse(message string) api.GetV2Clusters500JSONResponse {
	return api.GetV2Clusters500JSONResponse{
		N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
			Message: ptr(message),
		},
	}
}

// getClusters retrieves and processes the list of clusters in the specified namespace by calling
// the Cluster API (CAPI), returning a slice of ClusterInfo pointers or nil if an error occurs.
func (s *Server) getClusters(ctx context.Context, namespace string, orderBy, filter *string) (*[]api.ClusterInfo, error) {
	if namespace == "" {
		return nil, fmt.Errorf("no namespace provided")
	}

	clusters, err := fetchClustersList(ctx, s, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clusters: %w", err)
	}

	convertedClusters := s.convertClusters(ctx, namespace, clusters)

	if filter != nil {
		convertedClusters, err = FilterItems(convertedClusters, *filter, filterClusters)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filters: %w", err)
		}
	}

	if orderBy != nil {
		convertedClusters, err = OrderItems(convertedClusters, *orderBy, orderClustersBy)
		if err != nil {
			return nil, fmt.Errorf("failed to apply order by: %w", err)
		}
	}

	return &convertedClusters, nil
}

func (s *Server) convertClusters(ctx context.Context, namespace string, unstructuredClusters []unstructured.Unstructured) []api.ClusterInfo {
	clusters := make([]api.ClusterInfo, 0, len(unstructuredClusters))
	allMachines, err := fetchAllMachinesList(ctx, s, namespace)
	if err != nil {
		slog.Error("failed to fetch machines", "namespace", namespace, "error", err)
		return nil
	}
	for _, item := range unstructuredClusters {
		capiCluster := capi.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &capiCluster); err != nil {
			slog.Error("failed to convert unstructured to cluster, skipping...", "unstructured", item, "error", err)
			continue
		}

		slog.Debug("Processing cluster", "name", capiCluster.Name, "labels", capiCluster.Labels)
		// get machines associated with the cluster
		machines := getClusterMachines(allMachines, capiCluster.Name)

		labels := labels.Filter(capiCluster.Labels)
		unstructuredLabels := convert.MapStringToAny(labels)

		lp, errs := getClusterLifecyclePhase(&capiCluster)
		if len(errs) > 0 {
			slog.Debug("errors while building cluster lifecycle phase", "cluster", capiCluster.Name, "errors", errs)
		}

		clusterInfo := api.ClusterInfo{
			Name:                &capiCluster.Name,
			ProviderStatus:      getProviderStatus(&capiCluster),
			Labels:              &unstructuredLabels,
			KubernetesVersion:   getKubernetesVersion(&capiCluster),
			LifecyclePhase:      lp,
			ControlPlaneReady:   getControlPlaneReady(&capiCluster),
			InfrastructureReady: getInfrastructureReady(&capiCluster),
			NodeHealth:          getNodeHealth(&capiCluster, machines),
			NodeQuantity:        ptr(len(machines)),
		}

		if capiCluster.Spec.Topology != nil && capiCluster.Spec.Topology.Version != "" {
			clusterInfo.KubernetesVersion = &capiCluster.Spec.Topology.Version
		}

		if clusterInfo.Name == ptr("") || clusterInfo.Name == nil || clusterInfo.KubernetesVersion == nil {
			slog.Warn("skipping cluster with missing name or version", "name", clusterInfo.Name, "version", clusterInfo.KubernetesVersion)
			continue
		}

		clusters = append(clusters, clusterInfo)
	}
	return clusters
}

func filterClusters(cluster api.ClusterInfo, filter *Filter) bool {
	switch filter.Name {
	case "name":
		return MatchSubstring(cluster.Name, filter.Value)
	case "kubernetesVersion":
		return MatchSubstring(cluster.KubernetesVersion, filter.Value)
	case "providerStatus":
		if cluster.ProviderStatus != nil {
			return MatchSubstring(cluster.ProviderStatus.Message, filter.Value)
		}
	case "lifecyclePhase":
		if cluster.LifecyclePhase != nil {
			return MatchSubstring(cluster.LifecyclePhase.Message, filter.Value)
		}
	default:
		return false
	}
	return false
}

func orderClustersBy(cluster1, cluster2 api.ClusterInfo, orderBy *OrderBy) bool {
	switch orderBy.Name {
	case "name":
		if orderBy.IsDesc {
			return *cluster1.Name > *cluster2.Name
		}
		return *cluster1.Name < *cluster2.Name
	case "kubernetesVersion":
		if orderBy.IsDesc {
			return *cluster1.KubernetesVersion > *cluster2.KubernetesVersion
		}
		return *cluster1.KubernetesVersion < *cluster2.KubernetesVersion
	case "providerStatus":
		if orderBy.IsDesc {
			return *cluster1.ProviderStatus.Message > *cluster2.ProviderStatus.Message
		}
		return *cluster1.ProviderStatus.Message < *cluster2.ProviderStatus.Message
	case "lifecyclePhase":
		if orderBy.IsDesc {
			return *cluster1.LifecyclePhase.Message > *cluster2.LifecyclePhase.Message
		}
		return *cluster1.LifecyclePhase.Message < *cluster2.LifecyclePhase.Message
	default:
		return false
	}
}
