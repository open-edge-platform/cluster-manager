// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"log/slog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

// (GET /v2/clusters/summary)
func (s *Server) GetV2ClustersSummary(ctx context.Context, request api.GetV2ClustersSummaryRequestObject) (api.GetV2ClustersSummaryResponseObject, error) {
	slog.Debug("GetV2ClustersSummary")
	namespace := request.Params.Activeprojectid.String()
	unstructuredClusters, err := fetchClustersList(ctx, s, namespace)

	if k8serrors.IsInternalError(err) || err != nil {
		return api.GetV2ClustersSummary500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	clusters := s.convertClusters(ctx, namespace, unstructuredClusters)

	summary := api.ClusterSummary{}

	for _, cluster := range clusters {
		statuses := []string{
			string(*cluster.ControlPlaneReady.Indicator),
			string(*cluster.InfrastructureReady.Indicator),
			string(*cluster.LifecyclePhase.Indicator),
			string(*cluster.NodeHealth.Indicator),
			string(*cluster.ProviderStatus.Indicator),
		}

		status := summarizeStatus(statuses)

		switch status {
		case api.STATUSINDICATIONIDLE:
			summary.Ready++
		case api.STATUSINDICATIONERROR:
			summary.Error++
		case api.STATUSINDICATIONINPROGRESS:
			summary.InProgress++
		default:
			summary.Unknown++
		}
	}

	sum := summary.Error + summary.InProgress + summary.Ready + summary.Unknown
	total := int32(len(clusters))

	if sum != total {
		return api.GetV2ClustersSummary500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("Cluster summary count mismatch"),
			},
		}, nil
	}

	summary.TotalClusters = total

	return api.GetV2ClustersSummary200JSONResponse(summary), nil
}

func summarizeStatus(statuses []string) api.StatusIndicator {
	allReady := true
	anyError := false
	anyInProgress := false
	anyUnknown := false

	for _, status := range statuses {
		switch status {
		case string(api.STATUSINDICATIONIDLE):
			// Do nothing, let allReady stay true unless a non-"ready" is found.
		case string(api.STATUSINDICATIONERROR):
			anyError = true
			allReady = false
		case string(api.STATUSINDICATIONINPROGRESS):
			anyInProgress = true
			allReady = false
		default:
			anyUnknown = true
			allReady = false
		}
	}

	switch {
	case allReady:
		return api.STATUSINDICATIONIDLE
	case anyError:
		return api.STATUSINDICATIONERROR
	case anyInProgress:
		return api.STATUSINDICATIONINPROGRESS
	case anyUnknown:
		return api.STATUSINDICATIONUNSPECIFIED
	default:
		return api.STATUSINDICATIONUNSPECIFIED
	}
}
