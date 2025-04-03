// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

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

	err := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(activeProjectID).Delete(ctx, name, v1.DeleteOptions{})
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
