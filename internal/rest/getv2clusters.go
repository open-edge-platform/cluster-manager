// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"log/slog"
	"math"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/open-edge-platform/cluster-manager/internal/convert"
	"github.com/open-edge-platform/cluster-manager/internal/labels"
	. "github.com/open-edge-platform/cluster-manager/internal/pagination"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

const MaxClusters = math.MaxInt32 // Maximum value for int32

// (GET /v2/clusters)
func (s *Server) GetV2Clusters(ctx context.Context, request api.GetV2ClustersRequestObject) (api.GetV2ClustersResponseObject, error) {
	pageSize, offset, orderBy, filter, err := ValidateParams(request.Params)
	if err != nil {
		return badRequestGetClustersResponse(err.Error()), nil
	}
	namespace := request.Params.Activeprojectid.String()
	allClusters := s.getClusters(ctx, namespace, orderBy, filter)
	if allClusters == nil {
		return internalServerErrorGetClustersResponse("failed to retrieve clusters"), nil
	} else if len(*allClusters) == 0 {
		return api.GetV2Clusters200JSONResponse{
			Clusters:      allClusters,
			TotalElements: 0,
		}, nil
	}

	paginatedClustersList, err := PaginateItems(*allClusters, *pageSize, *offset)
	if err != nil {
		return internalServerErrorGetClustersResponse(err.Error()), nil
	}

	if len(*paginatedClustersList) > MaxClusters {
		return badRequestGetClustersResponse("number of clusters exceeds the maximum allowed"), nil
	}

	slog.Info("Clusters state read", "namespace", namespace, "count", len(*allClusters))

	return api.GetV2Clusters200JSONResponse{
		Clusters:      paginatedClustersList,
		TotalElements: int32(len(*allClusters)),
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
func (s *Server) getClusters(ctx context.Context, namespace string, orderBy, filter *string) *[]api.ClusterInfo {
	if namespace == "" {
		slog.Warn("failed to get clusters project id")
		return nil
	}

	clusters, err := fetchClustersList(ctx, s, namespace)
	if err != nil {
		slog.Error("failed to fetch clusters", "namespace", namespace, "error", err)
		return nil
	}

	convertedClusters := s.convertClusters(ctx, namespace, clusters)

	if filter != nil {
		convertedClusters, err = FilterItems(convertedClusters, *filter, filterClusters)
		if err != nil {
			slog.Error("failed to apply filters", "error", err)
			return nil
		}
	}

	if orderBy != nil {
		convertedClusters, err = OrderItems(convertedClusters, *orderBy, orderClustersBy)
		if err != nil {
			slog.Error("failed to apply order by", "error", err)
			return nil
		}
	}
	return &convertedClusters
}

func (s *Server) convertClusters(ctx context.Context, namespace string, unstructuredClusters []unstructured.Unstructured) []api.ClusterInfo {
	clusters := make([]api.ClusterInfo, 0, len(unstructuredClusters))
	for _, item := range unstructuredClusters {
		capiCluster := capi.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &capiCluster); err != nil {
			slog.Error("failed to convert unstructured to cluster, skipping...", "unstructured", item, "error", err)
			continue
		}

		slog.Debug("Processing cluster", "name", capiCluster.Name, "labels", capiCluster.Labels)
		// get machines associated with the cluster
		machines, err := fetchMachinesList(ctx, s, namespace, capiCluster.Name)
		if err != nil {
			slog.Error("failed to fetch machines for cluster", "cluster", capiCluster.Name, "error", err)
			continue
		}

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
		return MatchWildcard(cluster.Name, filter.Value)
	case "kubernetesVersion":
		return MatchWildcard(cluster.KubernetesVersion, filter.Value)
	case "providerStatus":
		if cluster.ProviderStatus != nil {
			return MatchWildcard(cluster.ProviderStatus.Message, filter.Value)
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
	default:
		return false
	}
}
